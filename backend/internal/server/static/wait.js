(function () {
  var el = document.getElementById("wait");
  var req = el.dataset.req;
  var statusEl = document.getElementById("status");
  var base = "/oauth/authorize/";

  function finish() {
    window.location.href = base + "complete?req=" + encodeURIComponent(req);
  }

  function poll() {
    fetch(base + "status?req=" + encodeURIComponent(req), { credentials: "same-origin", cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (d) {
        if (d.status === "pending_device") {
          statusEl.textContent = "En attente de votre confirmation… (" + d.expires_in + " s)";
          setTimeout(poll, 2000);
          return;
        }
        statusEl.textContent = d.status === "approved" ? "Identité confirmée. Redirection…" : "Demande terminée. Redirection…";
        finish();
      })
      .catch(function () { setTimeout(poll, 4000); });
  }

  poll();
})();
