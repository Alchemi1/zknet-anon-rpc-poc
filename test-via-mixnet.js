// Quick KPS -> mixnet connectivity test
// Run with: node test-via-mixnet.js <kps-addr>
// Example: node test-via-mixnet.js 127.0.0.1:9201:uEiABCD...

async function main() {
  const kpsAddr = process.argv[2] || "127.0.0.1:9201";
  console.log("Testing KPS connection to:", kpsAddr);

  try {
    // Dynamic import of KPS client
    const { dial } = await import("@kpstreams/quic-client");
    const conn = await dial(kpsAddr);
    const stream = await conn.openStream();

    const writer = stream.writable.getWriter();
    const request = JSON.stringify({
      jsonrpc: "2.0",
      method: "eth_blockNumber",
      params: [],
      id: 1,
    });
    await writer.write(new TextEncoder().encode(request));
    await writer.close();

    const reader = stream.readable.getReader();
    const decoder = new TextDecoder();
    let response = "";
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      response += decoder.decode(value, { stream: true });
    }

    console.log("Response:", response);
    const parsed = JSON.parse(response.replace(/^[^{]*/, ""));
    if (parsed.result) {
      console.log("Block number:", parseInt(parsed.result, 16));
      console.log("SUCCESS: KPS -> mixnet -> Ethereum works!");
      process.exit(0);
    }
  } catch (err) {
    console.error("FAILED:", err.message);
    console.log("");
    console.log("Make sure walletshield with KPS is running.");
    console.log("Usage: node test-via-mixnet.js <kps-addr>");
    console.log("Get KPS addr from: curl http://127.0.0.1:9200/boot");
    process.exit(1);
  }
}

main();
