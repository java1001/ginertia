import './app.css'
import { createInertiaApp, router } from '@inertiajs/svelte'
import Layout from './Layout.svelte'

// When the server clears history (ginertia.ClearHistory on logout), also drop
// prefetched pages; otherwise pressing Back can show a cached private page.
router.on('navigate', (event) => {
  if (event.detail.page.clearHistory) router.flushAll()
})

createInertiaApp({
  // "Users/Index" from ginertia.Render(...) → ./Pages/Users/Index.svelte
  resolve: (name) => {
    const pages = import.meta.glob('./Pages/**/*.svelte', { eager: true })
    const page = pages[`./Pages/${name}.svelte`]
    if (!page) throw new Error(`Page not found: ${name}`)
    return page
  },
  layout: () => Layout,
  progress: { color: '#e5484d' },
})
