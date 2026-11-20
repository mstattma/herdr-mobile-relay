const e = new URL(window.__HERDR_ENTRY__ || "/builds/0.25.0-490002-1e4ad2975e59c695/index.html", location);
  e.search = location.search;
  e.hash = location.hash;
  location.replace(e);
