import { defineConfig } from "astro/config";

export default defineConfig({
  build: {
    inlineStylesheets: "always",
  },
  compressHTML: false,
  devToolbar: {
    enabled: false,
  },
  vite: {
    build: {
      target: "es2022",
    },
  },
});
