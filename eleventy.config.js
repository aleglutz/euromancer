import figlet from "figlet";
import { execFileSync } from "node:child_process";
export default function(eleventyConfig) {
    const pathPrefix = "/euromancer/";
    eleventyConfig.addPassthroughCopy("css");
    eleventyConfig.addPassthroughCopy("assets");
    eleventyConfig.addPassthroughCopy("archive/**/attachments/**");
    eleventyConfig.addFilter("figlet", function (text, font = "Slant") {
        return figlet.textSync(text, { font });
});
    eleventyConfig.addFilter("maxcols", function (s) {
        return Math.max(...String(s).split("\n").map((l) => [...l].length));
});
    eleventyConfig.addFilter("tdfiglet", function (text, font = "corosv2x") {
        const render = (s) => execFileSync("tdfiglet", ["-f", `${font}.tdf`, "-w", "200", s], { encoding: "utf8" })
            .replace(/\x1b\[[0-9;]*m/g, "")   // ANSI-цвета не нужны в HTML
            .replace(/[ \t]+$/gm, "")
            .replace(/^\n+|\n+$/g, "");
        try {
            const parts = String(text).split("+");
            if (parts.length === 1) return render(text);
            // в tdf-шрифтах нет глифа "+" — рисуем свой и склеиваем построчно
            const PLUS = ["        ", "  ▄▄    ", "▄▄██▄▄  ", "▀▀██▀▀  ", "  ▀▀    "];
            const blocks = parts.map((p) => {
                const lines = render(p).split("\n");
                const w = Math.max(...lines.map((l) => l.length));
                return { lines, w };
            });
            const h = Math.max(PLUS.length, ...blocks.map((b) => b.lines.length));
            return Array.from({ length: h }, (_, i) =>
                blocks.map((b) => (b.lines[i] || "").padEnd(b.w + 2)).join(PLUS[i] || " ".repeat(8))
            ).join("\n").replace(/[ \t]+$/gm, "");
        } catch {
            // нет бинаря (или шрифта) — деградируем в classic figlet
            return figlet.textSync(text, { font: "Slant" });
        }
});
    eleventyConfig.addCollection("posts", function (collectionApi) {
        return collectionApi
        .getFilteredByGlob("archive/*/*.md")
        .reverse(); // свежие сверху
});
    eleventyConfig.addFilter("renderDate", function(date) {
        const d = date instanceof Date ? date : new Date();
        const opts = {
            weekday: 'short', month: 'short', day: '2-digit',
            hour: '2-digit', minute: '2-digit', second: '2-digit',
            hour12: false
        };
        return d.toLocaleString('en-US', opts);
    });
    eleventyConfig.addFilter("date", function (value, format = "iso") {
        if (!(value instanceof Date)) return value;
        if (format === "iso") {
            return value.toISOString().slice(0, 10);
  }
  // место для будущих форматов — "human", "long" и т.д.
  return value.toISOString().slice(0, 10);
});
    eleventyConfig.addTransform("wikilinks", function (content) {
  if (!this.page.outputPath || !this.page.outputPath.endsWith(".html")) {
    return content;
  }
    // this.page.url = "/archive/0001/CityNowhen/" (без pathPrefix)
    // собираем parentUrl без префикса, потом добавляем префикс
  const parentUrl = this.page.url.replace(/[^/]+\/?$/, "");
  const attachmentsUrl = pathPrefix.replace(/\/$/, "") + parentUrl + "attachments/";
  return content.replace(
  /!\[\[([^\]]+)\]\]/g,
  (_match, path) => {
    const filename = path.split("/").pop();
    return `<img src="${attachmentsUrl}${filename}" alt="">`;
  }
);
});
    eleventyConfig.addTransform("slides", function (content) {
  if (!this.page.outputPath || !this.page.outputPath.endsWith(".html")) {
    return content;
  }
  if (!content.includes("<!-- slide:")) {
    return content;
  }
  const zone = content.match(/<article class="slides">([\s\S]*?)<\/article>/);
  if (!zone) return content;
  const [fullMatch, inner] = zone;
  const parts = inner.split(/<!--\s*slide:(\w+)\s*-->/);
  // parts[0] is content before the first marker (e.g. the static cover from post.njk)
  let slides = parts[0];
  for (let i = 1; i < parts.length; i += 2) {
    const type = parts[i];
    const body = (parts[i + 1] || "").trim();
    slides += `<section class="slide slide--${type}">\n${body}\n</section>\n`;
  }
  const wrapped = `<article class="slides">\n${slides}</article>`;
  return content.replace(fullMatch, wrapped);
});
    eleventyConfig.addTransform("slide-numbers", function (content) {
  if (!this.page.outputPath || !this.page.outputPath.endsWith(".html")) return content;
  const slidePattern = /<section class="slide [^"]*">[\s\S]*?<\/section>/g;
  const allSlides = content.match(slidePattern);
  if (!allSlides) return content;
  const total = allSlides.length;
  const tt = String(total).padStart(2, "0");
  const prompt = `<span class="slide-prompt">n_euromancer@<span class="prompt-host">emag</span>:~<span class="prompt-sign">$</span></span>`;
  let counter = 0;
  return content.replace(slidePattern, (match) => {
    counter++;
    const nn = String(counter).padStart(2, "0");
    const openEnd = match.indexOf(">") + 1;
    const closeStart = match.lastIndexOf("</section>");
    return (
      match.slice(0, openEnd) +
      `\n${prompt}\n` +
      match.slice(openEnd, closeStart) +
      `<span class="slide-num">${nn}/${tt}</span>\n</section>`
    );
  });
});
  return {
    pathPrefix: pathPrefix,
    dir: {
      input: ".",
      output: "_site",
      includes: "_includes"
    },
    markdownTemplateEngine: "njk",
    htmlTemplateEngine: "njk"
  };
}