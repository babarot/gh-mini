// A shared worker holding the one connection to the server's event stream
// for every tab. A browser opens at most six HTTP/1.1 connections to a
// host, and with a stream per tab, six tabs left none for loading pages.

const ports = new Set();
const events = new EventSource("/_mini/events");

events.onmessage = (e) => {
  for (const port of ports) port.postMessage(e.data);
};

// A tab says "close" when it is left, and "open" when it comes back from
// the back/forward cache
onconnect = (e) => {
  const port = e.ports[0];
  ports.add(port);
  port.onmessage = (m) => {
    if (m.data === "close") ports.delete(port);
    else if (m.data === "open") ports.add(port);
  };
  port.start();
};
