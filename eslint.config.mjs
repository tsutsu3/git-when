// ESLint checks only the order of top-level declarations in the frontend.
// Prettier handles formatting.
// typescript-eslint needs the TypeScript 6 API, so package.json installs "typescript" as
// @typescript/typescript6. The tsc command still comes from TypeScript 7.

import perfectionist from "eslint-plugin-perfectionist";
import tseslint from "typescript-eslint";

export default [
  {
    files: ["frontend/src/**/*.ts"],
    languageOptions: {
      parser: tseslint.parser,
    },
    plugins: { perfectionist },
    rules: {
      // Types first, then exported functions, then private helpers.
      // The order inside each group is free.
      "perfectionist/sort-modules": [
        "error",
        {
          type: "unsorted",
          groups: [
            ["export-interface", "export-type"],
            ["interface", "type"],
            "export-function",
            "function",
          ],
        },
      ],
    },
  },
];
