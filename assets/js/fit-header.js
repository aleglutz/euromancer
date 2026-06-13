(function () {
  if (document.body.classList.contains('render-mode')) return;
  var pre = document.querySelector('header h1.post-figlet pre');
  if (!pre) return;
  var h1 = pre.closest('h1');
  var hdr = h1.parentElement;

  function fit() {
    var cs = getComputedStyle(hdr);
    var w = hdr.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight);
    // Reset to natural size to measure actual rendered art dimensions
    pre.style.transform = 'none';
    pre.style.fontSize = '20px';
    pre.style.width = 'max-content';
    var artW = pre.offsetWidth;
    var artH = pre.offsetHeight;
    pre.style.width = '';
    var scale = Math.min((w - 1) / artW, 1);
    pre.style.transformOrigin = '0 0';
    pre.style.transform = 'scale(' + scale + ')';
    h1.style.height = Math.ceil(artH * scale) + 'px';
  }

  (document.fonts ? document.fonts.ready : Promise.resolve()).then(fit);
  window.addEventListener('resize', fit);
}());
