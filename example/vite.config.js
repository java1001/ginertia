import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import ginertia from './vite-plugin-ginertia.js'

export default defineConfig({
  plugins: [
    svelte(),
    // Writes public/hot while `vite` runs so the Go side switches to the
    // dev server (HMR) automatically.
    ginertia({ input: 'frontend/app.js', ssr: 'frontend/ssr.js' }),
  ],
})
