<script>
  import { Link, useForm } from '@inertiajs/svelte'

  // user is nil (null) on /users/create, a User struct on /users/:id/edit
  let { user } = $props()

  const editing = $derived(!!user)
  const form = useForm(() => ({
    name: user?.name ?? '',
    email: user?.email ?? '',
    role: user?.role ?? 'viewer',
  }))

  function submit(e) {
    e.preventDefault()
    // Validation errors from ginertia.WithErrors land in form.errors.
    if (editing) form.put(`/users/${user.id}`)
    else form.post('/users')
  }
</script>

<svelte:head><title>{editing ? 'Edit user' : 'New user'} · Ginertia</title></svelte:head>

<div class="head">
  <h1>{editing ? `Edit ${user.name}` : 'New user'}</h1>
  <Link href="/users" class="btn ghost">Back</Link>
</div>

<form class="card form" onsubmit={submit} novalidate>
  <label>
    <span>Name</span>
    <input bind:value={form.name} class:invalid={form.errors.name} autocomplete="off" />
    {#if form.errors.name}<small class="err">{form.errors.name}</small>{/if}
  </label>

  <label>
    <span>Email</span>
    <input type="email" bind:value={form.email} class:invalid={form.errors.email} autocomplete="off" />
    {#if form.errors.email}<small class="err">{form.errors.email}</small>{/if}
  </label>

  <label>
    <span>Role</span>
    <select bind:value={form.role} class:invalid={form.errors.role}>
      <option value="admin">admin</option>
      <option value="editor">editor</option>
      <option value="viewer">viewer</option>
    </select>
    {#if form.errors.role}<small class="err">{form.errors.role}</small>{/if}
  </label>

  <div class="form-foot">
    <button class="btn" type="submit" disabled={form.processing}>
      {form.processing ? 'Saving…' : editing ? 'Save changes' : 'Create user'}
    </button>
    {#if form.hasErrors}<span class="err">Fix the errors above.</span>{/if}
  </div>
</form>
