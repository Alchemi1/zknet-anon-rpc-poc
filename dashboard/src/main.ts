import { invoke } from "@tauri-apps/api/core";

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

interface ProxyTestResult {
  success: boolean;
  response: string;
  duration_ms: number;
  error: string;
}

function statusDotClass(status: string): string {
  if (status.includes("Up")) return "up";
  if (status.includes("Restarting")) return "restarting";
  if (status === "down" || status === "error") return "down";
  return status.includes("Up") ? "up" : "down";
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
  if (epoch.consensus_status.includes("OK")) {
    dot.className = "status-dot online";
    text.textContent = "Network operational";
  } else if (epoch.consensus_status.includes("partial")) {
    dot.className = "status-dot partial";
    text.textContent = "Partial consensus";
  } else {
    dot.className = "status-dot offline";
    text.textContent = "No consensus";
  }
}

async function refreshAll() {
  document.getElementById("lastUpdated")!.textContent = "refreshing...";

  try {
    const containers = await invoke<ContainerInfo[]>("get_containers");
    renderContainers(containers);
  } catch (e) {
    console.error("Failed to get containers:", e);
  }

  try {
    const epoch = await invoke<EpochInfo>("get_epoch_status");
    renderEpoch(epoch);
  } catch (e) {
    console.error("Failed to get epoch status:", e);
  }

  const now = new Date();
  document.getElementById("lastUpdated")!.textContent = `Updated ${now.toLocaleTimeString()}`;
}

async function runProxyTest() {
  const btn = document.getElementById("testBtn") as HTMLButtonElement;
  const resultDiv = document.getElementById("testResult")!;
  btn.disabled = true;
  resultDiv.className = "test-result pending";
  resultDiv.textContent = "Sending request through mixnet (up to 120s)...";

  try {
    const result = await invoke<ProxyTestResult>("test_http_proxy", {
      url: "http://127.0.0.1:9205/",
    });

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
    resultDiv.textContent = `Dashboard error: ${e}`;
  }

  btn.disabled = false;
}

async function loadLogs(container: string) {
  const logArea = document.getElementById("logArea")!;
  logArea.innerHTML = "loading...";

  try {
    const lines = await invoke<string[]>("get_container_logs", {
      container,
      tail: 30,
    });
    logArea.innerHTML = lines
      .map((l) => {
        const timeMatch = l.match(/(\d{2}:\d{2}:\d{2})/);
        const time = timeMatch ? timeMatch[1] : "";
        const rest = time ? l.slice(timeMatch!.index! + 8) : l;
        return `<div class="log-line"><span class="time">${time}</span>${escapeHtml(rest)}</div>`;
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

  // Auto-refresh every 15s
  setInterval(refreshAll, 15000);
  setInterval(() => {
    const sel = document.getElementById("logSelector") as HTMLSelectElement;
    loadLogs(sel.value);
  }, 30000);

  // Expose functions globally for onclick handlers
  (window as any).refreshAll = refreshAll;
  (window as any).runProxyTest = runProxyTest;
});
