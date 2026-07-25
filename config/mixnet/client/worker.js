(()=>{var p="__KPS_ADDR__";anonRpcWorker.signalReady();function h(t,e){if(!e&&t.includes("ethereum"))return new TextEncoder().encode(JSON.stringify({jsonrpc:"2.0",method:"eth_blockNumber",params:[],id:1}));let n="";if(e&&e.body)try{n=typeof e.body=="string"?e.body:new TextDecoder().decode(e.body)}catch{n=JSON.stringify(e.body)}let o=e?`${e.method||"POST"} ${t} HTTP/1.1\r
Host: ethereum\r
Content-Type: application/json\r
Content-Length: ${n.length}\r
\r
${n}`:`GET ${t} HTTP/1.1\r
Host: ethereum\r
\r
`;return new TextEncoder().encode(o)}function u(t){let e=t.reduce((s,l)=>s+l.length,0),n=new Uint8Array(e),o=0;for(let s of t)n.set(s,o),o+=s.length;let r=new TextDecoder().decode(n),c=r.split(`\r
\r
`),a=c.length>1?c.slice(1).join(`\r
\r
`):r,i=new TextEncoder().encode(a),d=r.match(/HTTP\/\d\.\d\s+(\d+)/);return{status:d?parseInt(d[1]):200,headers:[["content-type","application/json"]],body:i}}async function y(){for(;;){let t=await anonRpcWorker.acceptCall();if(t.kind==="fetch")try{let e=await anonRpcWorker.kps.openStream(p),n=e.writable.getWriter(),o=e.readable.getReader();await n.write(h(t.url,t.requestInit)),await n.close();let r=[];for(;;){let{done:c,value:a}=await o.read();if(c)break;r.push(a)}t.respond(u(r))}catch(e){t.respond(Promise.reject(e))}}}y();})();
