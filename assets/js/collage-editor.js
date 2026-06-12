/* collage editor — активируется через ?edit=1 на странице поста.
   Крутит --tw-font / --tw-lh на .typewriting и --col/--row/--w на картинках,
   картинки таскаются мышью по символьной сетке. Выдаёт сниппет для .md. */
(() => {
  if (!new URLSearchParams(location.search).has("edit")) return;
  const tw = document.querySelector(".typewriting");
  if (!tw) return;

  const fonts = ["iA Writer Mono", "Erica Type", "Cascadia Mono", "IBM_VGA_8x16", "Robotron_A7100", "Menlo"];
  const imgs = [...tw.querySelectorAll("img")];

  /* стартовые значения — то, что реально отрендерено, а не зашитые дефолты */
  const cs = getComputedStyle(tw);
  const state = {
    font: cs.fontFamily.split(",")[0].replace(/['"]/g, "").trim(),
    lh: Math.round((parseFloat(cs.lineHeight) / parseFloat(cs.fontSize)) * 100) / 100 || 1.4,
    imgs: imgs.map((img) => ({
      el: img,
      src: img.getAttribute("src"),
      col: parseFloat(img.style.getPropertyValue("--col")) || 0,
      row: parseFloat(img.style.getPropertyValue("--row")) || 0,
      w: parseFloat(img.style.getPropertyValue("--w")) || 10,
    })),
  };

  function apply() {
    tw.style.setProperty("--tw-font", `'${state.font}'`);
    tw.style.setProperty("--tw-lh", state.lh);
    for (const it of state.imgs) {
      it.el.style.setProperty("--col", it.col);
      it.el.style.setProperty("--row", it.row);
      it.el.style.setProperty("--w", it.w);
    }
    out.textContent = snippet();
  }

  function snippet() {
    const imgTags = state.imgs.map(
      (it) => `<img src="${it.src}"\n     style="--col: ${it.col}; --row: ${it.row}; --w: ${it.w}">`
    );
    return [
      `<div class="typewriting" style="--tw-font: '${state.font}'; --tw-lh: ${state.lh}">`,
      `<pre>…</pre>`,
      ...imgTags,
      `</div>`,
    ].join("\n");
  }

  /* размеры ячейки сетки в px — для пересчёта drag в ch/lh */
  function cell() {
    const probe = document.createElement("span");
    probe.style.cssText = "position:absolute;visibility:hidden;width:1ch;height:1lh";
    tw.append(probe);
    const r = probe.getBoundingClientRect();
    probe.remove();
    return { ch: r.width, lh: r.height };
  }

  const css = document.createElement("style");
  css.textContent = `
    .twe { position: fixed; top: 12px; right: 12px; z-index: 99;
      background: var(--bg); color: var(--fg); border: 1px solid var(--fg);
      padding: 10px 12px; font: 12px/1.6 'Cascadia Mono', monospace; width: 230px; }
    .twe label { display: inline-block; width: 4.5em; color: var(--fg-faint); }
    .twe select, .twe input { font: inherit; background: var(--bg); color: var(--fg);
      border: 1px solid var(--border); }
    .twe input { width: 3.5em; }
    .twe fieldset { border: 1px dashed var(--border); margin: 8px 0; padding: 4px 6px; }
    .twe legend { color: var(--accent); padding: 0 4px; max-width: 100%; overflow: hidden;
      text-overflow: ellipsis; white-space: nowrap; }
    .twe pre { margin: 8px 0; padding: 6px; background: var(--bg-warm);
      font-size: 10px; line-height: 1.4; white-space: pre-wrap; word-break: break-all; }
    .twe button { font: inherit; background: var(--fg); color: var(--bg);
      border: 0; padding: 2px 10px; cursor: pointer; }
    .typewriting img { cursor: move; outline: 1px dashed var(--accent); }
  `;
  document.head.append(css);

  const panel = document.createElement("div");
  panel.className = "twe";
  panel.innerHTML = `
    <div><label>font</label><select name="font">${(fonts.includes(state.font) ? fonts : [state.font, ...fonts])
      .map((f) => `<option${f === state.font ? " selected" : ""}>${f}</option>`)
      .join("")}</select></div>
    <div><label>l-height</label><input name="lh" type="number" step="0.05" min="0.8" max="3" value="${state.lh}"></div>
    ${state.imgs
      .map(
        (it, i) => `<fieldset data-i="${i}">
        <legend>${it.src.split("/").pop()}</legend>
        <label>col</label><input name="col" type="number" step="1" value="${it.col}"><br>
        <label>row</label><input name="row" type="number" step="1" value="${it.row}"><br>
        <label>w</label><input name="w" type="number" step="1" min="1" value="${it.w}">
      </fieldset>`
      )
      .join("")}
    <pre></pre>
    <button>copy</button>
  `;
  document.body.append(panel);
  const out = panel.querySelector("pre");

  panel.addEventListener("input", (e) => {
    const f = e.target.closest("fieldset");
    const v = parseFloat(e.target.value);
    if (e.target.name === "font") state.font = e.target.value;
    else if (e.target.name === "lh" && !isNaN(v)) state.lh = v;
    else if (f && !isNaN(v)) state.imgs[f.dataset.i][e.target.name] = v;
    apply();
  });

  panel.querySelector("button").addEventListener("click", (e) => {
    navigator.clipboard.writeText(snippet());
    e.target.textContent = "copied";
    setTimeout(() => (e.target.textContent = "copy"), 1200);
  });

  state.imgs.forEach((it, i) => {
    it.el.addEventListener("pointerdown", (e) => {
      e.preventDefault();
      const c = cell();
      const start = { x: e.clientX, y: e.clientY, col: it.col, row: it.row };
      const move = (ev) => {
        it.col = Math.round(start.col + (ev.clientX - start.x) / c.ch);
        it.row = Math.round(start.row + (ev.clientY - start.y) / c.lh);
        const fs = panel.querySelector(`fieldset[data-i="${i}"]`);
        fs.querySelector('[name="col"]').value = it.col;
        fs.querySelector('[name="row"]').value = it.row;
        apply();
      };
      const up = () => {
        window.removeEventListener("pointermove", move);
        window.removeEventListener("pointerup", up);
      };
      window.addEventListener("pointermove", move);
      window.addEventListener("pointerup", up);
    });
  });

  apply();
})();
