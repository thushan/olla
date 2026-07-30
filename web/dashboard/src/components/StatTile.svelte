<script>
  let { label, value, sub, valueHtml = false, subHtml = false } = $props();
</script>

<div class="tile">
  <span class="label">{label}</span>
  {#if valueHtml}
    <!-- eslint-disable-next-line svelte/no-at-html-tags -->
    <span class="value">{@html value}</span>
  {:else}
    <span class="value">{value}</span>
  {/if}
  {#if sub}
    {#if subHtml}
      <!-- eslint-disable-next-line svelte/no-at-html-tags -->
      <span class="sub">{@html sub}</span>
    {:else}
      <span class="sub">{sub}</span>
    {/if}
  {/if}
</div>
