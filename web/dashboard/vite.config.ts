import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// https://vite.dev/config/
export default defineConfig({
  base: "/dashboard/",
  server: {
    proxy: {
      "/api": "http://127.0.0.1:1922",
    },
  },
  build: {
    outDir: "../../internal/comrad/dashboard_static",
    emptyOutDir: true,
  },
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
})
