<script>
  import { Link, page, router } from '@inertiajs/svelte'
  import { fly } from 'svelte/transition'

  let { children } = $props()

  // page.flash comes from ginertia.Flash(c, "success", "...") on the Go side.
  let toast = $state(null)
  let timer
  $effect(() =>
    router.on('flash', (e) => {
      toast = e.detail.flash.success ?? e.detail.flash.error ?? null
      clearTimeout(timer)
      timer = setTimeout(() => (toast = null), 3000)
    }),
  )

  const active = (prefix) =>
    prefix === '/' ? page.url === '/' : page.url.startsWith(prefix)
</script>

<header class="topbar">
  <div class="wrap topbar-inner">
    <Link href="/" class="brand">
      <span class="brand-mark">G</span>{page.props.appName}
    </Link>
    <nav>
      {#if page.props.auth.user}
        <!-- prefetch: the page is fetched on hover, so the click is instant -->
        <Link href="/" prefetch class={active('/') ? 'on' : ''}>Home</Link>
        <Link href="/feed" prefetch class={active('/feed') ? 'on' : ''}>Feed</Link>
        <Link href="/users" prefetch class={active('/users') ? 'on' : ''}>Users</Link>
      {/if}
      <a href="/docs">Inertia docs ↗</a>
    </nav>
    {#if page.props.auth.user}
      <span class="who">{page.props.auth.user.name}</span>
      <Link href="/logout" method="post" as="button" class="btn ghost sm">Log out</Link>
    {:else}
      <Link href="/login" class="btn ghost sm">Log in</Link>
    {/if}
  </div>
</header>

<main class="wrap">
  {@render children()}
</main>

{#if toast}
  <div class="toast" role="status" transition:fly={{ y: 16, duration: 180 }}>{toast}</div>
{/if}
