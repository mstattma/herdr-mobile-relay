const e = new URL(window.__HERDR_ENTRY__ || "/builds/0.24.0-485006-0be25ec6b0f27487/index.html", location);
  e.search = location.search;
  e.hash = location.hash;
  location.replace(e);
