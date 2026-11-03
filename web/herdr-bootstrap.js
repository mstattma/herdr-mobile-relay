const e = new URL(window.__HERDR_ENTRY__ || "/builds/0.24.0-485007-f2e218a202643bfa/index.html", location);
  e.search = location.search;
  e.hash = location.hash;
  location.replace(e);
