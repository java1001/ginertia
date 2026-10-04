<script>
  import { router, usePoll } from '@inertiajs/svelte'

  // Props passed from the Gin handler: ginertia.Render(c, "Home", gin.H{...})
  let { goVersion, serverTime } = $props()

  // Polling: partial reload of serverTime every 2s (stops when the tab is hidden).
  const poll = usePoll(2000, { only: ['serverTime'] })
</script>

<svelte:head><title>Home · Ginertia</title></svelte:head>

<section class="hero">
  <p class="eyebrow">Gin → Svelte, no API layer</p>
  <h1>Your Go handler's values are this page's props.</h1>
  <p class="lede">
    This page was rendered by <code>ginertia.Render(c, "Home", gin.H&#123;...&#125;)</code>.
    Navigation, forms and validation errors go through Inertia — no JSON endpoints to write.
  </p>
</section>

<div class="grid2">
  <div class="card">
    <div class="label">Go version (from server)</div>
    <div class="big mono">{goVersion}</div>
  </div>
  <div class="card">
    <div class="label">Server time</div>
    <div class="big mono">{serverTime}</div>
    <div class="row">
      <button class="btn ghost sm" onclick={() => router.reload({ only: ['serverTime'] })}>
        Partial reload
      </button>
      <button class="btn ghost sm" onclick={() => (poll.polling ? poll.stop() : poll.start())}>
        {poll.polling ? 'Stop polling' : 'Start polling'}
      </button>
    </div>
  </div>
</div>
