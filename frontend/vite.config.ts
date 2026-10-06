import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

// В разработке API и картинки отдаёт Go-бэкенд (go run . в backend/).
const backend = process.env.RP_BACKEND ?? "http://127.0.0.1:8080";

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: { "/api": backend, "/img": backend },
  },
});
