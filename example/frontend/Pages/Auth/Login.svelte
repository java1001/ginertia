<script>
  import { Form } from '@inertiajs/svelte'
</script>

<svelte:head><title>Log in · Ginertia</title></svelte:head>

<div class="auth">
  <h1>Log in</h1>
  <p class="muted">Any email works. Password: <code>secret</code></p>

  <!-- <Form> posts to the Gin route; errors from ginertia.WithErrors come back here. -->
  <Form action="/login" method="post" class="card form" resetOnError={['password']}>
    {#snippet children({ errors, processing })}
      <label>
        <span>Email</span>
        <input type="email" name="email" class:invalid={errors.email} autocomplete="username" />
        {#if errors.email}<small class="err">{errors.email}</small>{/if}
      </label>
      <label>
        <span>Password</span>
        <input type="password" name="password" class:invalid={errors.password} autocomplete="current-password" />
        {#if errors.password}<small class="err">{errors.password}</small>{/if}
      </label>
      <div class="form-foot">
        <button class="btn" type="submit" disabled={processing}>
          {processing ? 'Logging in…' : 'Log in'}
        </button>
      </div>
    {/snippet}
  </Form>
</div>
