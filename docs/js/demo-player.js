// Mounts the demo player on pages carrying the demo-player element.
document.addEventListener("DOMContentLoaded", function () {
  var el = document.getElementById("demo-player");
  if (el && window.AsciinemaPlayer) {
    AsciinemaPlayer.create(el.dataset.cast, el, {
      cols: 110, rows: 30, idleTimeLimit: 3, theme: "gruvbox-dark",
    });
  }
});
