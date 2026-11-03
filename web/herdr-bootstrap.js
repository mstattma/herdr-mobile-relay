const e = new URL(window.__HERDR_ENTRY__ || "/builds/0.24.0-485002-4e931734f10ed027/index.html", location);
  e.search = location.search;
  e.hash = location.hash;
  location.replace(e);
