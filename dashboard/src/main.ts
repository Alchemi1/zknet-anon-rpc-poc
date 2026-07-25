interface ContainerInfo {
  name: string;
  status: string;
  ports: string;
}

interface EpochInfo {
  current_epoch: string;
  consensus_status: string;
  last_consensus: string;
}

interface AnonRpcStatus {
  kps: { listening: boolean; boot: any; port: number };
  worker: { worker_ready: boolean; worker_hash: string; worker_size?: number; contract_exists: boolean; source_lines: number };
  http_proxy_port: number;
  kps_port: number;
  walletshield_port: number;
  kps_addr: string;
  worker_hash: string;
}

interface ProxyTestResult {
  success: boolean;
  response: string;
  duration_ms: number;
  error: string;
}

interface LogLine {
  time: string;
  text: string;
}

const API_BASE = "";

async function api<T>(path: string, method = "GET"): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, { method });
  if (!res.ok) throw new Error(`API error: ${res.status}`);
  return res.json();
}

function statusDotClass(status: string): string {
  if (status.includes("Up")) return "up";
  if (status.includes("Restarting")) return "restarting";
  return "down";
}

function renderContainers(containers: ContainerInfo[]) {
  const grid = document.getElementById("containersGrid")!;
  grid.innerHTML = containers
    .map(
      (c) => `
    <div class="container-card">
      <span class="dot ${statusDotClass(c.status)}"></span>
      <div class="info">
        <div class="name">${c.name}</div>
        <div class="status">${c.status}</div>
      </div>
    </div>`
    )
    .join("");
}

function renderEpoch(epoch: EpochInfo) {
  document.getElementById("currentEpoch")!.textContent = epoch.current_epoch || "N/A";
  document.getElementById("consensusStatus")!.textContent = epoch.consensus_status;
  document.getElementById("lastConsensus")!.textContent = epoch.last_consensus || "N/A";

  const dot = document.getElementById("statusDot")!;
  const text = document.getElementById("statusText")!;
  if (epoch.consensus_status.includes("OK") && !epoch.consensus_status.includes("no")) {
    dot.className = "status-dot online";
    text.textContent = "Network operational";
  } else if (epoch.consensus_status.includes("partial") || epoch.consensus_status.includes("(")) {
    dot.className = "status-dot partial";
    text.textContent = "Partial consensus";
  } else {
    dot.className = "status-dot offline";
    text.textContent = "No consensus";
  }
}

function renderAnonRpc(data: AnonRpcStatus) {
  document.getElementById("wsHttp")!.textContent = data.kps.listening ? `online (:${data.kps.port})` : "offline";
  document.getElementById("wsHttp")!.style.color = data.kps.listening ? "var(--green)" : "var(--red)";

  document.getElementById("kpsStatus")!.textContent = data.kps.listening ? data.kps_addr : "not running";
  document.getElementById("kpsStatus")!.style.color = data.kps.listening ? "var(--green)" : "var(--muted)";

  const w = data.worker;
  if (w.worker_ready) {
    document.getElementById("workerStatus")!.innerHTML = `✅ built (${(w.worker_size! / 1024).toFixed(0)}KB, ${w.source_lines} src lines)`;
    document.getElementById("workerHash")!.textContent = (data.worker_hash || w.worker_hash).slice(0, 42) + "...";
  } else {
    document.getElementById("workerStatus")!.textContent = "not built";
    document.getElementById("workerStatus")!.style.color = "var(--yellow)";
    document.getElementById("workerHash")!.textContent = "-";
  }
}

async function refreshAll() {
  document.getElementById("lastUpdated")!.textContent = "refreshing...";

  try {
    const containers = await api<ContainerInfo[]>("/api/containers");
    renderContainers(containers);
  } catch (e) {
    console.error("Failed to get containers:", e);
  }

  try {
    const epoch = await api<EpochInfo>("/api/epoch");
    renderEpoch(epoch);
  } catch (e) {
    console.error("Failed to get epoch status:", e);
  }

  try {
    const anon = await api<AnonRpcStatus>("/api/anonrpc");
    renderAnonRpc(anon);
  } catch (e) {
    console.error("Failed to get anon-rpc status:", e);
  }

  const now = new Date();
  document.getElementById("lastUpdated")!.textContent = `Updated ${now.toLocaleTimeString()}`;
}

async function runProxyTest() {
  const btn = document.getElementById("testBtn") as HTMLButtonElement;
  const resultDiv = document.getElementById("testResult")!;
  btn.disabled = true;
  resultDiv.className = "test-result pending";
  resultDiv.textContent = "Sending request through mixnet (~5-120s)...";

  try {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 130000);
    const res = await fetch("/api/test", {
      method: "POST",
      signal: controller.signal,
    });
    clearTimeout(timeoutId);

    if (!res.ok) throw new Error(`HTTP ${res.status}`);

    const result: ProxyTestResult = await res.json();

    if (result.success) {
      let pretty: string;
      try {
        pretty = JSON.stringify(JSON.parse(result.response), null, 2);
      } catch {
        pretty = result.response;
      }
      resultDiv.className = "test-result success";
      resultDiv.innerHTML = `${pretty}\n\n<span style="color:var(--muted)">(${result.duration_ms}ms)</span>`;
    } else {
      resultDiv.className = "test-result failure";
      const errMsg = result.error ? `\nError: ${result.error}` : "";
      const resp = result.response ? `\nResponse: ${result.response}` : "";
      resultDiv.textContent = `Request failed${errMsg}${resp}\n(${result.duration_ms}ms)`;
    }
  } catch (e) {
    resultDiv.className = "test-result failure";
    resultDiv.textContent = `Request failed: ${e}`;
  }

  btn.disabled = false;
}

async function loadLogs(container: string) {
  const logArea = document.getElementById("logArea")!;
  logArea.innerHTML = "loading...";

  try {
    const lines = await api<LogLine[]>(`/api/logs/${encodeURIComponent(container)}`);
    logArea.innerHTML = lines
      .map((l) => {
        const time = l.time || "";
        const rest = time ? (l.text.startsWith(time) ? l.text.slice(time.length) : l.text) : l.text;
        return `<div class="log-line"><span class="time">${escapeHtml(time)}</span>${escapeHtml(rest)}</div>`;
      })
      .join("");
  } catch (e) {
    logArea.textContent = `Error loading logs: ${e}`;
  }
}

function escapeHtml(text: string): string {
  const div = document.createElement("div");
  div.textContent = text;
  return div.innerHTML;
}

// Initialize
document.addEventListener("DOMContentLoaded", () => {
  const logSelector = document.getElementById("logSelector") as HTMLSelectElement;
  logSelector.addEventListener("change", () => loadLogs(logSelector.value));

  refreshAll();
  loadLogs(logSelector.value);

  setInterval(refreshAll, 15000);
  setInterval(() => {
    const sel = document.getElementById("logSelector") as HTMLSelectElement;
    loadLogs(sel.value);
  }, 30000);

  (window as any).refreshAll = refreshAll;
  (window as any).runProxyTest = runProxyTest;
});

