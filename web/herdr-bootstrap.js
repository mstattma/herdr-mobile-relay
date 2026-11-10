const e = new URL(window.__HERDR_ENTRY__ || "/builds/0.25.0-490001-de9bf11702007fd9/index.html", location);
  e.search = location.search;
  e.hash = location.hash;
  location.replace(e);
