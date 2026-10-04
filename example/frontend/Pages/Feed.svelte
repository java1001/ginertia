<script>
  import { InfiniteScroll } from '@inertiajs/svelte'

  // posts: ginertia.ScrollPage(items, page, hasMore) on the Go side.
  let { posts, total } = $props()
</script>

<svelte:head><title>Feed · Ginertia</title></svelte:head>

<div class="head">
  <h1>Feed</h1>
  <span class="muted"><b class="count">{posts.data.length}</b> / {total} loaded</span>
</div>

<InfiniteScroll data="posts" class="feed" buffer={200}>
  {#each posts.data as post (post.id)}
    <article class="card post" data-id={post.id}>
      <h3>{post.title}</h3>
      <p class="muted">{post.body}</p>
    </article>
  {/each}

  {#snippet loading()}
    <div class="muted feed-status">Loading more…</div>
  {/snippet}
</InfiniteScroll>

{#if posts.data.length >= total}
  <p class="muted feed-status end">You've reached the end.</p>
{/if}
