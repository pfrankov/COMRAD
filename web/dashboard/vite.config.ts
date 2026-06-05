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
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) {
            return
          }
          if (
            id.includes("/radix-ui/") ||
            id.includes("/cmdk/") ||
            id.includes("/sonner/")
          ) {
            return "ui-vendor"
          }
          if (id.includes("/lucide-react/")) {
            return "icons-vendor"
          }
          return "vendor"
        },
      },
    },
  },
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
})
