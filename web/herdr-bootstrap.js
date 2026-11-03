const e = new URL(window.__HERDR_ENTRY__ || "/builds/0.24.0-485004-d53d5cfccc85d02e/index.html", location);
  e.search = location.search;
  e.hash = location.hash;
  location.replace(e);
