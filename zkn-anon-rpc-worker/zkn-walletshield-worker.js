// Anon-RPC compliant worker for ZKNetwork WalletShield
// KPS -> walletshield -> Katzenpost mixnet -> Ethereum
// Complies with anon-rpc SPEC.md v0.1.0

const KPS_ADDR = "__KPS_ADDR__";
const ETH_RPC_URL = "/ethereum-rpc";

anonRpcWorker.signalReady();

function encodeRequest(url, init) {
  if (!init && url.includes("ethereum")) {
    return new TextEncoder().encode(
      JSON.stringify({ jsonrpc: "2.0", method: "eth_blockNumber", params: [], id: 1 })
    );
  }
  let body = "";
  if (init && init.body) {
    try { body = typeof init.body === "string" ? init.body : new TextDecoder().decode(init.body); }
    catch { body = JSON.stringify(init.body); }
  }
  const req = init
    ? `${init.method || "POST"} ${url} HTTP/1.1\r\nHost: ethereum\r\nContent-Type: application/json\r\nContent-Length: ${body.length}\r\n\r\n${body}`
    : `GET ${url} HTTP/1.1\r\nHost: ethereum\r\n\r\n`;
  return new TextEncoder().encode(req);
}

function decodeResponse(chunks) {
  const total = chunks.reduce((acc, c) => acc + c.length, 0);
  const combined = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    combined.set(chunk, offset);
    offset += chunk.length;
  }
  const text = new TextDecoder().decode(combined);
  const parts = text.split("\r\n\r\n");
  const bodyStr = parts.length > 1 ? parts.slice(1).join("\r\n\r\n") : text;
  let bodyBytes = new TextEncoder().encode(bodyStr);
  const statusMatch = text.match(/HTTP\/\d\.\d\s+(\d+)/);
  return {
    status: statusMatch ? parseInt(statusMatch[1]) : 200,
    headers: [["content-type", "application/json"]],
    body: bodyBytes,
  };
}

async function main() {
  while (true) {
    const call = await anonRpcWorker.acceptCall();
    if (call.kind !== "fetch") continue;

    try {
      const stream = await anonRpcWorker.kps.openStream(KPS_ADDR);
      const writer = stream.writable.getWriter();
      const reader = stream.readable.getReader();

      await writer.write(encodeRequest(call.url, call.requestInit));
      await writer.close();

      const chunks = [];
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        chunks.push(value);
      }

      call.respond(decodeResponse(chunks));
    } catch (err) {
      call.respond(Promise.reject(err));
    }
  }
}

main();
