/**
 * Builds internal/render/template.html, the single-file HTML report that Go embeds.
 *
 * The build runs these steps in order.
 *   1. Run the Astro build into frontend/dist.
 *   2. Compile sample.ts into a plain script.
 *   3. Put the sample script between the Go data markers.
 *   4. Inline the bundled application script.
 *   5. Minify the HTML.
 *   6. Verify that the result is one self-contained file that Go can fill.
 *
 * With --check, the script compares the result with the committed template instead of writing it.
 */

import { execFile } from "node:child_process";
import { readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";

import { transform } from "esbuild";
import { minify } from "html-minifier-terser";

const run = promisify(execFile);
const frontendDir = dirname(fileURLToPath(import.meta.url));
const repositoryDir = resolve(frontendDir, "..");
const distDir = resolve(frontendDir, "dist");
const outputPath = resolve(repositoryDir, "internal/render/template.html");
const astroBin = resolve(repositoryDir, "node_modules/.bin/astro");

// index.astro marks where the sample data goes.
const SAMPLE_MARKER = "<!--@git-when-sample-->";
// Go replaces everything between these markers with the real data (see internal/render/html.go).
const DATA_START = "<!--@git-when-data-start-->";
const DATA_END = "<!--@git-when-data-end-->";
// The GitHub link in the top bar is the only allowed URL.
// A link loads nothing until the viewer follows it, so the report still works offline.
const GITHUB_URL = "https://github.com/tsutsu3/git-when";

const count = (text, part) => text.split(part).length - 1;

/** Step 1. Runs the Astro build and returns the built page. */
async function runAstroBuild() {
  await run(astroBin, ["build", "--root", frontendDir], { cwd: repositoryDir });
  return readFile(resolve(distDir, "index.html"), "utf8");
}

/** Step 2. Compiles sample.ts into minified JavaScript that runs as a classic script. */
async function compileSample() {
  const source = await readFile(resolve(frontendDir, "src/scripts/sample.ts"), "utf8");
  const result = await transform(source, {
    charset: "utf8",
    format: "iife",
    legalComments: "none",
    loader: "ts",
    minify: true,
    target: "es2022",
  });
  if (/<\/script/i.test(result.code)) {
    throw new Error("sample.ts: compiled JavaScript contains a closing script tag");
  }
  return result.code.trim();
}

/** Step 3. Replaces the sample marker with the sample script wrapped in the Go data markers. */
function insertDataMarkers(html, sample) {
  if (count(html, SAMPLE_MARKER) !== 1) {
    throw new Error(`Astro output must contain exactly one ${SAMPLE_MARKER}`);
  }
  return html.replace(SAMPLE_MARKER, () => `${DATA_START}<script>${sample}</script>${DATA_END}`);
}

/** Step 4. Replaces each bundled script reference with the script content. */
async function inlineAstroAssets(html) {
  const scriptPattern = /<script\b([^>]*?)\bsrc="([^"]+)"([^>]*)><\/script>/g;
  let bundled = html;

  for (const match of [...html.matchAll(scriptPattern)]) {
    const source = match[2];
    if (/^(?:[a-z]+:)?\/\//i.test(source)) {
      throw new Error(`external script is not allowed in the report: ${source}`);
    }
    const assetPath = resolve(distDir, source.replace(/^\/+/, ""));
    const code = (await readFile(assetPath, "utf8")).trim();
    if (/<\/script/i.test(code)) {
      throw new Error(`${source}: bundled JavaScript contains a closing script tag`);
    }
    const attributes = `${match[1]}${match[3]}`.trim();
    const replacement = `<script${attributes ? ` ${attributes}` : ""}>${code}</script>`;
    bundled = bundled.replace(match[0], () => replacement);
  }

  return bundled;
}

/** Step 5. Minifies the HTML and inline CSS. The data markers are kept. */
async function minifyHtml(html) {
  const minified = await minify(html, {
    collapseBooleanAttributes: true,
    collapseWhitespace: true,
    decodeEntities: true,
    keepClosingSlash: true,
    minifyCSS: true,
    minifyJS: false,
    removeAttributeQuotes: true,
    removeComments: true,
    removeEmptyAttributes: true,
    removeOptionalTags: false,
    removeRedundantAttributes: true,
    sortAttributes: true,
    sortClassName: true,
    useShortDoctype: true,
    ignoreCustomComments: [/^@git-when-data-(?:start|end)$/],
  });
  return `${minified}\n`;
}

/**
 * Step 6. Checks the contract with Go and the offline guarantee.
 *
 * The page must have one data script between the markers and one application script outside them.
 * It must not load anything from outside the file.
 */
function verifyTemplate(html) {
  const problems = [];

  const descriptions = html.match(
    /<meta\b(?=[^>]*\bname=["']?description(?:["'\s>]))(?=[^>]*\bcontent=["'][^"']+["'])[^>]*>/gi,
  );
  if (descriptions?.length !== 1) {
    problems.push("the page must have exactly one non-empty meta description");
  }

  const japaneseLanguageButton =
    /<button\b(?=[^>]*\bdata-lang-choice=["']?ja(?:["'\s>]))(?=[^>]*\baria-label=["'][^"']*\bJA\b[^"']*["'])[^>]*>\s*JA\s*<\/button>/i;
  if (!japaneseLanguageButton.test(html)) {
    problems.push("the Japanese language button accessible name must include JA");
  }

  const start = html.indexOf(DATA_START);
  const end = html.indexOf(DATA_END);
  if (count(html, DATA_START) !== 1 || count(html, DATA_END) !== 1 || end < start) {
    problems.push("the data markers must appear exactly once each, start before end");
  } else {
    const dataBlock = html.slice(start + DATA_START.length, end);
    const outside = html.slice(0, start) + html.slice(end + DATA_END.length);
    if (count(dataBlock, "<script") !== 1) {
      problems.push("the data block must hold exactly one script");
    }
    if (count(outside, "<script") !== 1) {
      problems.push("the page must have exactly one application script outside the data block");
    }
  }

  for (const match of html.matchAll(/\s(src|href)=["']?([^"'\s>]+)/gi)) {
    const isGitHubLink = match[1].toLowerCase() === "href" && match[2] === GITHUB_URL;
    if (!/^(?:data:|#)/i.test(match[2]) && !isGitHubLink) {
      problems.push(`the page references a file or URL: ${match[0].trim()}`);
    }
  }
  // The only allowed <link> is the favicon, and the href check above requires a data URI.
  for (const link of html.matchAll(/<link\b[^>]*>/gi)) {
    if (!/\srel=["']?icon\b/i.test(link[0])) {
      problems.push(`the page has a <link> that is not the favicon: ${link[0].slice(0, 80)}`);
    }
  }
  if (/@import\b/i.test(html)) problems.push("the CSS has an @import rule");
  if (/url\(\s*["']?(?!data:|#)/i.test(html)) problems.push("the CSS loads a url()");
  if (html.includes("/_astro/")) problems.push("the page still references an Astro asset");

  if (problems.length) {
    throw new Error(`template check failed:\n  ${problems.join("\n  ")}`);
  }
}

async function buildTemplate() {
  const [astroHtml, sample] = await Promise.all([runAstroBuild(), compileSample()]);
  const withData = insertDataMarkers(astroHtml, sample);
  const bundled = await inlineAstroAssets(withData);
  const output = await minifyHtml(bundled);
  verifyTemplate(output);
  return output;
}

const output = await buildTemplate();
if (process.argv.includes("--check")) {
  const current = await readFile(outputPath, "utf8").catch(() => "");
  if (current !== output) {
    console.error("internal/render/template.html is stale; run pnpm build:template");
    process.exitCode = 1;
  }
} else {
  await writeFile(outputPath, output);
  console.log(`built ${outputPath} (${Buffer.byteLength(output)} bytes)`);
}
