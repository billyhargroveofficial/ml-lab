import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  base: '/ml/',
  plugins: [react()],
  server: {
    port: 5190,
    proxy: { '/ml/api': 'http://127.0.0.1:17490', '/ml/data': 'http://127.0.0.1:17490' },
  },
});
