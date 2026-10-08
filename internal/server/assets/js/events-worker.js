// A shared worker holding the one connection to the server's event stream
// for every tab. A browser opens at most six HTTP/1.1 connections to a
// host, and with a stream per tab, six tabs left none for loading pages.

const ports = new Set();
const events = new EventSource("/_mini/events");

events.onmessage = (e) => {
  for (const port of ports) port.postMessage(e.data);
};

// Not kept for tabs that come later: while the stream is down, the last
// one may be from a server that is gone, and a tab from the new one would
// reload until the stream is back
events.addEventListener("boot", (e) => {
  const msg = JSON.stringify({ boot: e.data });
  for (const port of ports) port.postMessage(msg);
});

// A tab says "close" when it is left, and "open" when it comes back from
// the back/forward cache. Once it is passed the stream again it is told
// so, and asks for what it missed before: told any sooner, it could miss
// a change that came in between
const attach = (port) => {
  ports.add(port);
  port.postMessage(JSON.stringify({ attached: true }));
};
onconnect = (e) => {
  const port = e.ports[0];
  port.onmessage = (m) => {
    if (m.data === "close") ports.delete(port);
    else if (m.data === "open") attach(port);
  };
  port.start();
  attach(port);
};
