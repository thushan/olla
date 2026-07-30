<script>
  import Header from './Header.svelte';
  import StatusStrip from './StatusStrip.svelte';
  import NavTabs from './NavTabs.svelte';

  let { children } = $props();
</script>

<header class="site-header">
  <Header />
  <StatusStrip />
  <NavTabs />
</header>

<main>
  {@render children?.()}
</main>

<footer>
  <span>Olla is a local, self-hosted proxy. This view never leaves your network.</span>
  <span>Olla by <a class="footer-link" href="https://tensorfoundry.io" target="_blank" rel="noopener noreferrer">TensorFoundry</a></span>
</footer>
