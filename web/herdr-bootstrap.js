const e = new URL(window.__HERDR_ENTRY__ || "/builds/0.24.0-485001-94e33ec0d606bddb/index.html", location);
  e.search = location.search;
  e.hash = location.hash;
  location.replace(e);
