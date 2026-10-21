(function () {
  // Boutons « Copier » : data-copy="texte" ou data-copy-block (copie le bloc de code voisin).
  document.querySelectorAll(".copy").forEach(function (btn) {
    btn.addEventListener("click", function () {
      var text = btn.getAttribute("data-copy");
      if (btn.hasAttribute("data-copy-block")) {
        var pre = btn.parentElement.querySelector("pre");
        text = pre ? pre.textContent : "";
      }
      if (!text || !navigator.clipboard) return;
      navigator.clipboard.writeText(text.trim()).then(function () {
        var old = btn.textContent;
        btn.textContent = "Copié";
        btn.classList.add("done");
        setTimeout(function () { btn.textContent = old; btn.classList.remove("done"); }, 1600);
      });
    });
  });

  // Onglets de code (Node.js, PHP, Python).
  document.querySelectorAll("[data-tabs]").forEach(function (group) {
    var tabs = group.querySelectorAll("[data-tab]");
    tabs.forEach(function (tab) {
      tab.addEventListener("click", function () {
        var name = tab.getAttribute("data-tab");
        tabs.forEach(function (t) { t.setAttribute("aria-selected", String(t === tab)); });
        group.querySelectorAll("[data-panel]").forEach(function (panel) {
          panel.hidden = panel.getAttribute("data-panel") !== name;
        });
      });
    });
  });
})();
