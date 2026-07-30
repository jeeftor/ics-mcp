import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: fileURLToPath(new URL('../internal/icsmcp/web/dist', import.meta.url)),
    emptyOutDir: true,
    assetsDir: 'assets'
  }
});
