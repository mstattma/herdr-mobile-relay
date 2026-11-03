const e = new URL(window.__HERDR_ENTRY__ || "/builds/0.24.0-485005-178555154f9d7f71/index.html", location);
  e.search = location.search;
  e.hash = location.hash;
  location.replace(e);
