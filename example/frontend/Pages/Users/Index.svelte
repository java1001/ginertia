<script>
  import { Link, Deferred, router } from '@inertiajs/svelte'

  // users: ginertia.Merge(...)  → appended on "Load more"
  // stats: ginertia.Defer(...)  → arrives in a second request
  let { users, page, hasMore, stats } = $props()

  let loading = $state(false)

  function loadMore() {
    router.reload({
      data: { page: page + 1 },
      only: ['users', 'page', 'hasMore'],
      onStart: () => (loading = true),
      onFinish: () => (loading = false),
    })
  }

  function destroy(user) {
    if (confirm(`Delete ${user.name}?`)) {
      router.delete(`/users/${user.id}`, { preserveScroll: true })
    }
  }

  const fmt = (iso) => new Date(iso).toLocaleDateString()
</script>

<svelte:head><title>Users · Ginertia</title></svelte:head>

<div class="head">
  <h1>Users</h1>
  <Link href="/users/create" class="btn">New user</Link>
</div>

<div class="stats">
  <Deferred data="stats">
    {#snippet fallback()}
      <div class="stat skeleton"></div>
      <div class="stat skeleton"></div>
      <div class="stat skeleton"></div>
      <div class="stat skeleton"></div>
    {/snippet}
    <div class="stat"><span>Total</span><b>{stats.total}</b></div>
    {#each Object.entries(stats.byRole) as [role, n]}
      <div class="stat"><span>{role}</span><b>{n}</b></div>
    {/each}
  </Deferred>
</div>

<div class="card flush">
  <table>
    <thead>
      <tr><th>Name</th><th>Email</th><th>Role</th><th>Joined</th><th></th></tr>
    </thead>
    <tbody>
      {#each users as user (user.id)}
        <tr>
          <td class="strong">{user.name}</td>
          <td class="muted">{user.email}</td>
          <td><span class="pill {user.role}">{user.role}</span></td>
          <td class="muted mono">{fmt(user.createdAt)}</td>
          <td class="actions">
            <Link href="/users/{user.id}/edit" class="btn ghost sm">Edit</Link>
            <button class="btn danger sm" onclick={() => destroy(user)}>Delete</button>
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

<div class="more">
  {#if hasMore}
    <button class="btn ghost" onclick={loadMore} disabled={loading}>
      {loading ? 'Loading…' : 'Load more'}
    </button>
  {:else}
    <span class="muted">All {users.length} users loaded</span>
  {/if}
</div>
