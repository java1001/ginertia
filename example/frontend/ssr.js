// SSR entry: `npm run build` then `npm run ssr`, and start Go with SSR=1.
import { createInertiaApp } from '@inertiajs/svelte'
import createServer from '@inertiajs/svelte/server'
import { render } from 'svelte/server'
import Layout from './Layout.svelte'

createServer((page) =>
  createInertiaApp({
    page,
    resolve: (name) => {
      const pages = import.meta.glob('./Pages/**/*.svelte', { eager: true })
      return pages[`./Pages/${name}.svelte`]
    },
    layout: () => Layout,
    setup: ({ App, props }) => render(App, { props }),
  }),
)
