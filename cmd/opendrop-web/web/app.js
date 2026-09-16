"use strict";

const $ = (id) => document.getElementById(id);
const queue = []; // File objects
let sending = false;

function humanBytes(n) {
  if (n < 1024) return n + " B";
  const units = ["KB", "MB", "GB", "TB"];
  let v = n, e = -1;
  while (v >= 1024 && e < units.length - 1) { v /= 1024; e++; }
  return v.toFixed(1) + " " + units[e];
}

async function loadInfo() {
  const r = await fetch("/api/info");
  const info = await r.json();
  $("device").textContent = info.device_name + " → LAN";
}

async function loadPeers() {
  $("peer-status").textContent = "scanning…";
  const sel = $("peer");
  const prev = sel.value;
  try {
    const r = await fetch("/api/peers");
    const data = await r.json();
    sel.innerHTML = "";
    const peers = data.peers || [];
    if (peers.length === 0) {
      const o = document.createElement("option");
      o.value = "";
      o.textContent = "(no peers found)";
      sel.appendChild(o);
      $("peer-status").textContent = "none — is the other daemon running?";
    } else {
      for (const p of peers) {
        const o = document.createElement("option");
        o.value = p.addr;
        o.textContent = p.name + " (" + p.addr + ")" + (p.ok ? "" : " — unreachable");
        sel.appendChild(o);
      }
      if (prev) sel.value = prev;
      const ok = peers.filter((p) => p.ok).length;
      $("peer-status").textContent = ok + "/" + peers.length + " reachable";
    }
  } catch (e) {
    $("peer-status").textContent = "scan failed: " + e.message;
  }
  updateSendButton();
}

function updateSendButton() {
  $("send").disabled = sending || queue.length === 0 || !$("peer").value;
}

function renderQueue() {
  const ul = $("queue");
  ul.innerHTML = "";
  queue.forEach((f, i) => {
    const li = document.createElement("li");
    li.className = "file";
    li.id = "q-" + i;
    li.innerHTML =
      '<div class="top"><span class="name"></span>' +
      '<span class="size">' + humanBytes(f.size) + "</span>" +
      '<button type="button" class="remove" data-i="' + i + '" title="Remove">✕</button></div>' +
      '<progress value="0" max="100" hidden></progress>' +
      '<div class="result"></div>';
    li.querySelector(".name").textContent = f.name;
    ul.appendChild(li);
  });
  ul.querySelectorAll(".remove").forEach((b) => {
    b.addEventListener("click", () => {
      if (sending) return;
      queue.splice(Number(b.dataset.i), 1);
      renderQueue();
      updateSendButton();
    });
  });
  updateSendButton();
}

function addFiles(files) {
  for (const f of files) queue.push(f);
  renderQueue();
}

function setProgress(i, pct) {
  const bar = document.querySelector("#q-" + i + " progress");
  if (!bar) return;
  bar.hidden = false;
  bar.value = pct;
}

function setResult(i, ok, text) {
  const el = document.querySelector("#q-" + i + " .result");
  if (!el) return;
  el.className = "result " + (ok ? "ok" : "err");
  el.textContent = text;
}

// One XHR per file so each gets its own progress bar.
function sendOne(to, file, i) {
  return new Promise((resolve) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/send?to=" + encodeURIComponent(to));
    xhr.upload.addEventListener("progress", (e) => {
      if (e.lengthComputable) setProgress(i, Math.round((e.loaded / e.total) * 100));
    });
    xhr.addEventListener("load", () => {
      setProgress(i, 100);
      try {
        const data = JSON.parse(xhr.responseText);
        const res = (data.results || [])[0];
        if (xhr.status === 200 && res && !res.error) {
          setResult(i, true, "sent (" + humanBytes(res.bytes) + ")");
          resolve(true);
        } else {
          setResult(i, false, (res && res.error) || ("HTTP " + xhr.status));
          resolve(false);
        }
      } catch (e) {
        setResult(i, false, "bad response: " + xhr.responseText.slice(0, 120));
        resolve(false);
      }
    });
    xhr.addEventListener("error", () => {
      setResult(i, false, "network error");
      resolve(false);
    });
    const form = new FormData();
    form.append("files", file, file.name);
    xhr.send(form);
  });
}

async function sendAll() {
  const to = $("peer").value;
  if (!to || queue.length === 0) return;
  sending = true;
  updateSendButton();
  $("send-status").textContent = "sending…";
  // Clear previous results.
  document.querySelectorAll("#queue .result").forEach((el) => { el.textContent = ""; });
  let ok = 0;
  for (let i = 0; i < queue.length; i++) {
    if (await sendOne(to, queue[i], i)) ok++;
  }
  $("send-status").textContent = ok + "/" + queue.length + " sent";
  sending = false;
  updateSendButton();
  loadInbox();
}

async function loadInbox() {
  $("inbox-status").textContent = "loading…";
  const ul = $("inbox");
  try {
    const r = await fetch("/api/inbox");
    const data = await r.json();
    $("inbox-dir").textContent = data.dir || "";
    ul.innerHTML = "";
    for (const f of data.files || []) {
      const li = document.createElement("li");
      li.className = "inbox-item";
      const a = document.createElement("a");
      a.href = "/files/" + encodeURIComponent(f.name);
      a.textContent = f.name;
      const size = document.createElement("span");
      size.className = "muted small";
      size.textContent = humanBytes(f.size);
      li.appendChild(a);
      li.appendChild(size);
      ul.appendChild(li);
    }
    $("inbox-status").textContent = (data.files || []).length + " files";
  } catch (e) {
    $("inbox-status").textContent = "failed: " + e.message;
  }
}

function init() {
  const dz = $("dropzone");
  const input = $("file-input");

  ["dragenter", "dragover"].forEach((ev) =>
    dz.addEventListener(ev, (e) => { e.preventDefault(); dz.classList.add("over"); }));
  ["dragleave", "drop"].forEach((ev) =>
    dz.addEventListener(ev, (e) => { e.preventDefault(); dz.classList.remove("over"); }));
  dz.addEventListener("drop", (e) => {
    if (e.dataTransfer && e.dataTransfer.files.length) addFiles(e.dataTransfer.files);
  });
  dz.addEventListener("keydown", (e) => {
    if (e.key === "Enter" || e.key === " ") input.click();
  });
  input.addEventListener("change", () => {
    addFiles(input.files);
    input.value = "";
  });

  $("send").addEventListener("click", sendAll);
  $("clear").addEventListener("click", () => {
    if (sending) return;
    queue.length = 0;
    renderQueue();
    $("send-status").textContent = "";
  });
  $("refresh-peers").addEventListener("click", loadPeers);
  $("refresh-inbox").addEventListener("click", loadInbox);
  $("peer").addEventListener("change", updateSendButton);

  loadInfo();
  loadPeers();
  loadInbox();
}

document.addEventListener("DOMContentLoaded", init);
