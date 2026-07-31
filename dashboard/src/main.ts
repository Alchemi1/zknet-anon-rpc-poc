import { Wallet } from "ethers";

interface ContainerInfo {
  name: string;
  status: string;
  ports: string;
}

interface ContainersResponse {
  containers: ContainerInfo[];
  active: number;
  total: number;
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

interface KpsProbe {
  success: boolean;
  rtt_ms: number;
  at: string;
  err?: string;
}

interface KpsStats {
  connected: boolean;
  kps_addr: string;
  uptime_seconds: number;
  last_probe_at: string;
  last_rtt_ms: number;
  last_result: string;
  min_rtt_ms: number;
  max_rtt_ms: number;
  avg_rtt_ms: number;
  success_count: number;
  error_count: number;
  success_rate: number;
  last_error: string;
  history: KpsProbe[];
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

function renderContainers(resp: ContainersResponse) {
  const grid = document.getElementById("containersGrid")!;
  grid.innerHTML = resp.containers
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

  document.getElementById("activeCount")!.textContent = `${resp.active}`;
  document.getElementById("totalCount")!.textContent = `${resp.total}`;
  const pct = resp.total > 0 ? Math.round(resp.active / resp.total * 100) : 0;
  document.getElementById("healthPct")!.textContent = `${pct}%`;
  document.getElementById("healthPct")!.style.color = pct === 100 ? "var(--green)" : pct >= 66 ? "var(--yellow)" : "var(--red)";
}

function renderEpoch(epoch: EpochInfo) {
  document.getElementById("currentEpoch")!.textContent = epoch.current_epoch || "N/A";
  document.getElementById("consensusStatus")!.textContent = epoch.consensus_status;
  document.getElementById("lastConsensus")!.textContent = epoch.last_consensus || "N/A";

  const dot = document.getElementById("statusDot")!;
  const text = document.getElementById("statusText")!;
  if (epoch.consensus_status.includes("consensus OK")) {
    dot.className = "status-dot online pulse";
    text.textContent = "Network operational";
  } else if (epoch.consensus_status.includes("partial")) {
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
    document.getElementById("workerStatus")!.innerHTML = `\u2705 built (${(w.worker_size! / 1024).toFixed(0)}KB, ${w.source_lines} src lines)`;
    document.getElementById("workerHash")!.textContent = (data.worker_hash || w.worker_hash).slice(0, 42) + "...";
  } else {
    document.getElementById("workerStatus")!.textContent = "not built";
    document.getElementById("workerStatus")!.style.color = "var(--yellow)";
    document.getElementById("workerHash")!.textContent = "-";
  }
}

function renderKpsStats(data: KpsStats) {
  const statusEl = document.getElementById("kpsTransportStatus")!;
  const connEl = document.getElementById("kpsConnStatus")!;
  const addrEl = document.getElementById("kpsAddr")!;
  const rttEl = document.getElementById("kpsLastRTT")!;
  const blockEl = document.getElementById("kpsBlockNumber")!;
  const rttStatsEl = document.getElementById("kpsRTTStats")!;
  const rateEl = document.getElementById("kpsSuccessRate")!;
  const countEl = document.getElementById("kpsProbeCount")!;
  const uptimeEl = document.getElementById("kpsUptime")!;
  const chartEl = document.getElementById("kpsRTTChart")!;

  if (!data.connected) {
    statusEl.textContent = "\u274c disconnected";
    connEl.textContent = "disconnected";
    connEl.style.color = "var(--red)";
    addrEl.textContent = data.kps_addr || "-";
    rttEl.textContent = "-";
    rttStatsEl.textContent = "-";
    rateEl.textContent = "-";
    countEl.textContent = "-";
    uptimeEl.textContent = "-";
    chartEl.innerHTML = "";
    return;
  }

  statusEl.textContent = "\u2705 connected";
  connEl.textContent = "connected";
  connEl.style.color = "var(--green)";
  addrEl.textContent = data.kps_addr;

  const lastRTT = data.last_rtt_ms || 0;
  rttEl.textContent = lastRTT > 0 ? `${lastRTT}ms` : "pending...";
  rttEl.style.color = lastRTT > 0 && lastRTT < 10000 ? "var(--green)" : "var(--yellow)";
  blockEl.textContent = data.last_result || "-";
  blockEl.style.color = data.last_result ? "var(--green)" : "var(--muted)";

  rttStatsEl.textContent = `${data.min_rtt_ms || 0} / ${data.avg_rtt_ms || 0} / ${data.max_rtt_ms || 0} ms`;
  rateEl.textContent = `${data.success_rate}%`;
  rateEl.style.color = data.success_rate >= 80 ? "var(--green)" : data.success_rate >= 50 ? "var(--yellow)" : "var(--red)";
  countEl.textContent = `${data.success_count} ok, ${data.error_count} err`;
  uptimeEl.textContent = `${Math.floor(data.uptime_seconds / 60)}m ${data.uptime_seconds % 60}s`;

  // RTT bar chart
  const maxRTT = Math.max(...data.history.map((h) => h.rtt_ms), 1);
  chartEl.innerHTML = data.history
    .map((h) => {
      const pct = Math.max(2, (h.rtt_ms / maxRTT) * 100);
      const color = h.success ? "var(--green)" : "var(--red)";
      return `<div style="flex:1;height:${pct}%;background:${color};border-radius:2px 2px 0 0;" title="${h.rtt_ms}ms ${h.success ? 'OK' : 'FAIL'}"></div>`;
    })
    .join("");
}

async function refreshAll() {
  document.getElementById("lastUpdated")!.textContent = "refreshing...";

  try {
    const containers = await api<ContainersResponse>("/api/containers");
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

  try {
    const kpsStats = await api<KpsStats>("/api/kps-stats");
    renderKpsStats(kpsStats);
  } catch (e) {
    console.error("Failed to get kps stats:", e);
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

async function sendKpsRpc() {
  const btn = document.getElementById("kpsRpcBtn") as HTMLButtonElement;
  const resultDiv = document.getElementById("kpsRpcResult")!;
  const method = (document.getElementById("kpsRpcMethod") as HTMLInputElement).value;
  const params = (document.getElementById("kpsRpcParams") as HTMLInputElement).value;

  btn.disabled = true;
  resultDiv.style.display = "block";
  resultDiv.className = "test-result pending";
  resultDiv.textContent = "Sending via KPS...";

  let parsed: any;
  try { parsed = JSON.parse(params); } catch { parsed = params; }

  const body = JSON.stringify({
    jsonrpc: "2.0", method, params: Array.isArray(parsed) ? parsed : [], id: 1
  });

  try {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 65000);
    const res = await fetch("/api/kps-rpc", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body, signal: controller.signal,
    });
    clearTimeout(timeoutId);
    const data = await res.json();
    if (data.error) {
      resultDiv.className = "test-result failure";
      resultDiv.textContent = `Error: ${data.error}`;
      if (data.rtt_ms) resultDiv.textContent += ` (${data.rtt_ms}ms)`;
    } else {
      resultDiv.className = "test-result success";
      let pretty = data.result;
      try { pretty = JSON.stringify(JSON.parse(data.result), null, 2); } catch {}
      resultDiv.innerHTML = `<pre style="margin:0;font-size:12px;white-space:pre-wrap;">${escapeHtml(pretty)}</pre><div class="duration">${data.rtt_ms}ms via KPS</div>`;
    }
  } catch (e) {
    resultDiv.className = "test-result failure";
    resultDiv.textContent = `Request failed: ${e}`;
  }
  btn.disabled = false;
}

async function kpsRpcCall(method: string, params: any[], timeoutMs = 90000): Promise<any> {
  const body = JSON.stringify({ jsonrpc: "2.0", method, params, id: 1 });
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const res = await fetch("/api/kps-rpc", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body, signal: controller.signal,
    });
    const data = await res.json();
    if (data.error) throw new Error(data.error);
    return data;
  } finally {
    clearTimeout(timeoutId);
  }
}

async function sendPrivateBroadcast() {
  const btn = document.getElementById("bcBtn") as HTMLButtonElement;
  const resultDiv = document.getElementById("bcResult")!;
  const keyInput = (document.getElementById("bcPrivateKey") as HTMLInputElement).value.trim();
  const toInput = (document.getElementById("bcTo") as HTMLInputElement).value.trim();
  const valueInput = (document.getElementById("bcValue") as HTMLInputElement).value.trim();

  btn.disabled = true;
  resultDiv.style.display = "block";
  resultDiv.className = "test-result pending";
  resultDiv.textContent = "Preparing...";

  try {
    if (!keyInput) throw new Error("private key required");
    if (!toInput.match(/^0x[a-fA-F0-9]{40}$/)) throw new Error("invalid to address");

    const key = keyInput.startsWith("0x") ? keyInput : "0x" + keyInput;
    const wallet = new Wallet(key);
    const value = BigInt(valueInput || "0");
    resultDiv.textContent = `Signed locally from ${wallet.address}. Fetching nonce + gas via mixnet...`;

    const [nonceData, gasData, chainData] = await Promise.all([
      kpsRpcCall("eth_getTransactionCount", [wallet.address, "pending"]),
      kpsRpcCall("eth_gasPrice", []),
      kpsRpcCall("eth_chainId", []),
    ]);
    const nonce = BigInt(nonceData.result);
    const gasPrice = BigInt(gasData.result);
    const chainId = Number(BigInt(chainData.result));

    resultDiv.textContent = `nonce=${nonce} gasPrice=${gasPrice} chainId=${chainId}. Signing locally...`;

    const rawTx = await wallet.signTransaction({
      to: toInput,
      value,
      nonce,
      gasPrice,
      gasLimit: 21000,
      chainId,
      type: 0,
    });

    resultDiv.textContent = "Broadcasting signed tx through mixnet...";
    const start = Date.now();
    const txResult = await kpsRpcCall("eth_sendRawTransaction", [rawTx], 120000);
    const rtt = Date.now() - start;
    const hash = txResult.result;
    resultDiv.className = "test-result success";
    resultDiv.innerHTML = `<pre style="margin:0;font-size:12px;white-space:pre-wrap;">tx hash: ${escapeHtml(hash)}</pre><div class="duration">${rtt}ms via mixnet · <a href="https://sepolia.etherscan.io/tx/${escapeHtml(hash)}" target="_blank" rel="noopener">view on Etherscan</a></div>`;
  } catch (e: any) {
    resultDiv.className = "test-result failure";
    resultDiv.textContent = `Broadcast failed: ${e.message || e}`;
    if ((e.message || "").includes("insufficient funds")) {
      resultDiv.textContent += "\n(Note: the signed tx DID reach the node via the mixnet — fund the sender on Sepolia and retry.)";
    }
  }
  btn.disabled = false;
}

let mwTimer: ReturnType<typeof setInterval> | null = null;
let mwHistory: number[] = [];

async function toggleMempoolWatch() {
  const btn = document.getElementById("mwBtn") as HTMLButtonElement;
  const countEl = document.getElementById("mwCount")!;
  const rttEl = document.getElementById("mwRTT")!;
  const updatedEl = document.getElementById("mwUpdated")!;
  const chartEl = document.getElementById("mwChart")!;

  if (mwTimer) {
    clearInterval(mwTimer);
    mwTimer = null;
    btn.textContent = "Start Watching";
    btn.style.background = "var(--blue)";
    return;
  }

  btn.textContent = "Watching...";
  btn.style.background = "var(--red)";

  const poll = async () => {
    try {
      const data = await kpsRpcCall("eth_getBlockTransactionCountByNumber", ["pending"], 45000);
      const count = Number(BigInt(data.result));
      countEl.textContent = `${count}`;
      countEl.style.color = count > 0 ? "var(--green)" : "var(--muted)";
      rttEl.textContent = `${data.rtt_ms}ms`;
      updatedEl.textContent = new Date().toLocaleTimeString();
      mwHistory.push(count);
      if (mwHistory.length > 30) mwHistory.shift();
      const maxC = Math.max(...mwHistory, 1);
      chartEl.innerHTML = mwHistory.map(c => {
        const pct = Math.max(2, (c / maxC) * 100);
        return `<div style="flex:1;height:${pct}%;background:var(--green);border-radius:1px 1px 0 0;" title="${c} pending"></div>`;
      }).join("");
    } catch (e: any) {
      countEl.textContent = "error";
      countEl.style.color = "var(--red)";
      rttEl.textContent = (e.message || e).slice(0, 60);
    }
  };

  mwHistory = [];
  chartEl.innerHTML = "";
  poll();
  mwTimer = setInterval(poll, 10000);
}

const CR_CALLDATA: Record<string, string> = {
  totalSupply: "0x18160ddd",
  symbol: "0x95d89b41",
  name: "0x06fdde03",
  decimals: "0x313ce567",
};

function crBalanceOfData(wallet: string): string {
  return "0x70a08231" + "000000000000000000000000" + wallet.slice(2).toLowerCase();
}

function decodeAbiString(hex: string): string {
  const h = hex.replace(/^0x/, "");
  if (h.length < 128) return "";
  const offset = parseInt(h.slice(0, 64), 16);
  if (isNaN(offset) || offset === 0) return "";
  const lenPos = 64 + offset * 2 - 64;
  const lenBytes = parseInt(h.slice(lenPos, lenPos + 64), 16);
  if (isNaN(lenBytes) || lenBytes === 0) return "";
  const dataStart = lenPos + 64;
  const strHex = h.slice(dataStart, dataStart + lenBytes * 2);
  let out = "";
  for (let i = 0; i < strHex.length; i += 2) {
    const code = parseInt(strHex.slice(i, i + 2), 16);
    if (code === 0) break;
    out += String.fromCharCode(code);
  }
  return out;
}

function decodeUint(hex: string): string {
  const h = hex.replace(/^0x/, "");
  if (h.length < 64) return "0";
  return BigInt("0x" + h.slice(0, 64)).toString();
}

async function contractRead(kind: string) {
  const btn = document.getElementById(kind === "balanceOf" ? "crBalanceBtn" :
    kind === "totalSupply" ? "crSupplyBtn" : kind === "symbol" ? "crSymbolBtn" :
    kind === "name" ? "crNameBtn" : "crDecimalsBtn") as HTMLButtonElement;
  const resultDiv = document.getElementById("crResult")!;
  const addr = (document.getElementById("crAddress") as HTMLInputElement).value.trim();
  const wallet = (document.getElementById("crWallet") as HTMLInputElement).value.trim();

  btn.disabled = true;
  resultDiv.style.display = "block";
  resultDiv.className = "test-result pending";
  resultDiv.textContent = `Reading ${kind} from ${addr} via mixnet...`;

  try {
    if (!addr.match(/^0x[a-fA-F0-9]{40}$/)) throw new Error("invalid contract address");
    let data: string;
    let extra = "";
    if (kind === "balanceOf") {
      if (!wallet.match(/^0x[a-fA-F0-9]{40}$/)) throw new Error("invalid wallet address");
      data = crBalanceOfData(wallet);
      extra = ` (wallet ${wallet})`;
    } else {
      data = CR_CALLDATA[kind];
    }
    const res = await kpsRpcCall("eth_call", [{ to: addr, data }, "latest"], 90000);
    const raw = res.result.replace(/^0x/, "").replace(/0+$/, "");
    resultDiv.className = "test-result success";

    if (kind === "symbol" || kind === "name") {
      const str = decodeAbiString(res.result);
      resultDiv.innerHTML = `<pre style="margin:0;font-size:13px;">${escapeHtml(str)}</pre><div class="duration">${res.rtt_ms}ms via mixnet</div>`;
    } else if (kind === "decimals") {
      resultDiv.innerHTML = `<pre style="margin:0;font-size:13px;">${decodeUint(res.result)}</pre><div class="duration">${res.rtt_ms}ms via mixnet</div>`;
    } else {
      resultDiv.innerHTML = `<pre style="margin:0;font-size:13px;">${decodeUint(res.result)} (raw)</pre><div class="duration">${res.rtt_ms}ms via mixnet</div>`;
    }
  } catch (e: any) {
    resultDiv.className = "test-result failure";
    resultDiv.textContent = `Read failed: ${e.message || e}`;
  }
  btn.disabled = false;
}

async function lookupTx() {
  const btn = document.getElementById("ttBtn") as HTMLButtonElement;
  const resultDiv = document.getElementById("ttResult")!;
  const hash = (document.getElementById("ttHash") as HTMLInputElement).value.trim();

  btn.disabled = true;
  resultDiv.style.display = "block";
  resultDiv.className = "test-result pending";
  resultDiv.textContent = "Looking up via mixnet...";

  try {
    if (!hash.match(/^0x[a-fA-F0-9]{64}$/)) throw new Error("invalid tx hash");
    const res = await kpsRpcCall("eth_getTransactionByHash", [hash], 90000);
    const tx = res.result;
    if (!tx || tx === null) {
      resultDiv.className = "test-result failure";
      resultDiv.textContent = `No transaction found for ${hash}`;
      return;
    }
    resultDiv.className = "test-result success";
    const rows = [
      ["block", tx.blockNumber],
      ["from", tx.from],
      ["to", tx.to || "(contract creation)"],
      ["value", BigInt(tx.value || "0x0").toString() + " wei"],
      ["gas", tx.gas],
      ["gasPrice", tx.gasPrice],
    ];
    resultDiv.innerHTML = rows.map(([k, v]) =>
      `<div style="display:flex;gap:8px;padding:3px 0;border-bottom:1px solid #1c1c1c;font-size:12px;"><span style="color:var(--muted);min-width:60px;">${k}</span><span style="font-family:monospace;word-break:break-all;">${escapeHtml(String(v))}</span></div>`
    ).join("") + `<div class="duration">${res.rtt_ms}ms via mixnet</div>`;
  } catch (e: any) {
    resultDiv.className = "test-result failure";
    resultDiv.textContent = `Lookup failed: ${e.message || e}`;
  }
  btn.disabled = false;
}

async function simulateTx() {
  const btn = document.getElementById("simBtn") as HTMLButtonElement;
  const resultDiv = document.getElementById("simResult")!;
  const from = (document.getElementById("simFrom") as HTMLInputElement).value.trim();
  const to = (document.getElementById("simTo") as HTMLInputElement).value.trim();
  const value = (document.getElementById("simValue") as HTMLInputElement).value.trim() || "0";
  const data = (document.getElementById("simData") as HTMLInputElement).value.trim() || "0x";

  btn.disabled = true;
  resultDiv.style.display = "block";
  resultDiv.className = "test-result pending";
  resultDiv.textContent = "Estimating gas via mixnet...";

  try {
    if (!from.match(/^0x[a-fA-F0-9]{40}$/)) throw new Error("invalid from address");
    if (!to.match(/^0x[a-fA-F0-9]{40}$/)) throw new Error("invalid to address");
    const txObj: any = { from, to, value: value.startsWith("0x") ? value : "0x" + BigInt(value).toString(16) };
    if (data !== "0x") txObj.data = data;
    const res = await kpsRpcCall("eth_estimateGas", [txObj], 90000);
    resultDiv.className = "test-result success";
    resultDiv.innerHTML = `<pre style="margin:0;font-size:13px;">${BigInt(res.result).toString()} gas units</pre><div class="duration">${res.rtt_ms}ms via mixnet · ${BigInt(res.result).toString()} gas</div>`;
  } catch (e: any) {
    resultDiv.className = "test-result failure";
    resultDiv.textContent = `Estimate failed: ${e.message || e}`;
  }
  btn.disabled = false;
}

let bwTimer: ReturnType<typeof setInterval> | null = null;
let bwHistory: { balance: number; rtt: number }[] = [];

async function toggleBalanceWatch() {
  const btn = document.getElementById("bwBtn") as HTMLButtonElement;
  const address = (document.getElementById("bwAddress") as HTMLInputElement).value.trim();
  const balanceEl = document.getElementById("bwBalance")!;
  const rttEl = document.getElementById("bwRTT")!;
  const updatedEl = document.getElementById("bwUpdated")!;
  const chartEl = document.getElementById("bwChart")!;

  if (bwTimer) {
    clearInterval(bwTimer);
    bwTimer = null;
    btn.textContent = "Start Watching";
    btn.style.background = "var(--blue)";
    return;
  }

  if (!address.match(/^0x[a-fA-F0-9]{40}$/)) {
    balanceEl.textContent = "invalid address";
    return;
  }

  btn.textContent = "Watching...";
  btn.style.background = "var(--red)";

  const poll = async () => {
    const body = JSON.stringify({
      jsonrpc: "2.0", method: "eth_getBalance",
      params: [address, "latest"], id: 1
    });
    try {
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 30000);
      const res = await fetch("/api/kps-rpc", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body, signal: controller.signal,
      });
      clearTimeout(timeoutId);
      const data = await res.json();
      if (data.result && !data.error) {
        const wei = BigInt(data.result);
        const eth = Number(wei) / 1e18;
        const display = eth >= 0.01 ? eth.toFixed(4) : eth.toFixed(8);
        balanceEl.textContent = `${display} ETH`;
        balanceEl.style.color = "var(--green)";
        rttEl.textContent = `${data.rtt_ms}ms`;
        updatedEl.textContent = new Date().toLocaleTimeString();
        bwHistory.push({ balance: eth, rtt: data.rtt_ms || 0 });
        if (bwHistory.length > 30) bwHistory.shift();
        const maxB = Math.max(...bwHistory.map(h => h.balance), 0.001);
        chartEl.innerHTML = bwHistory.map(h => {
          const pct = Math.max(2, (h.balance / maxB) * 100);
          return `<div style="flex:1;height:${pct}%;background:var(--green);border-radius:1px 1px 0 0;" title="${h.balance.toFixed(4)} ETH ${h.rtt}ms"></div>`;
        }).join("");
      } else {
        balanceEl.textContent = data.error || "error";
        balanceEl.style.color = "var(--red)";
        rttEl.textContent = data.rtt_ms ? `${data.rtt_ms}ms` : "-";
      }
    } catch (e) {
      balanceEl.textContent = "request failed";
      balanceEl.style.color = "var(--red)";
    }
  };

  bwHistory = [];
  chartEl.innerHTML = "";
  poll();
  bwTimer = setInterval(poll, 10000);
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
  (window as any).sendKpsRpc = sendKpsRpc;
  (window as any).sendPrivateBroadcast = sendPrivateBroadcast;
  (window as any).toggleMempoolWatch = toggleMempoolWatch;
  (window as any).contractRead = contractRead;
  (window as any).lookupTx = lookupTx;
  (window as any).simulateTx = simulateTx;
  (window as any).toggleBalanceWatch = toggleBalanceWatch;
});
