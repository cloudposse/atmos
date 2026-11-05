import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import vm from "node:vm";
import test from "node:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";

const require = createRequire(import.meta.url);
const docusaurusRequire = createRequire(
  require.resolve("@docusaurus/core/package.json"),
);
const babelRequire = createRequire(
  docusaurusRequire.resolve("@docusaurus/babel"),
);
const { transformSync } = babelRequire("@babel/core");
const filename = fileURLToPath(
  new URL("../../pages/examples-detail-design.tsx", import.meta.url),
);
const { code } = transformSync(readFileSync(filename, "utf8"), {
  filename,
  babelrc: false,
  configFile: false,
  sourceMaps: "inline",
  presets: [
    babelRequire.resolve("@babel/preset-typescript"),
    [babelRequire.resolve("@babel/preset-react"), { runtime: "classic" }],
    [
      babelRequire.resolve("@babel/preset-env"),
      { targets: { node: "current" }, modules: "commonjs" },
    ],
  ],
});

function renderPreview(examples) {
  const module = { exports: {} };
  const options = { routeBasePath: "/examples" };
  vm.runInNewContext(
    code,
    {
      module,
      exports: module.exports,
      require(name) {
        if (name === "react") return React;
        if (name.endsWith("file-browser-tree-examples.json"))
          return { examples };
        if (name.endsWith("file-browser-options-examples.json")) return options;
        if (name.endsWith("/ExampleDetailPage")) {
          return ({ example, options: receivedOptions, preview }) => {
            assert.equal(receivedOptions, options);
            assert.equal(preview, true);
            return React.createElement("span", null, example.name);
          };
        }
        throw new Error(`Unexpected dependency: ${name}`);
      },
    },
    { filename },
  );
  return renderToStaticMarkup(React.createElement(module.exports.default));
}

test("design preview selects the preferred example even when it is not first", () => {
  assert.equal(
    renderPreview([{ name: "other" }, { name: "quick-start-simple" }]),
    "<span>quick-start-simple</span>",
  );
});

test("design preview falls back to the first available example", () => {
  assert.equal(
    renderPreview([{ name: "first" }, { name: "second" }]),
    "<span>first</span>",
  );
});

test("design preview renders nothing when the collection is empty", () => {
  assert.equal(renderPreview([]), "");
});
