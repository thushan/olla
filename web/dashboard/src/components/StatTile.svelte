<script>
  // Rich value/sub content is delivered as Svelte snippets (`children`,
  // `subSnippet`) rather than raw HTML strings. The previous raw-HTML value/
  // sub props were an opt-in XSS sink on a shared component - inert while
  // only closed server-controlled values flowed through it, but a footgun the
  // moment an endpoint or model name did. Snippets compose into the DOM as
  // normal, escaped content.
  let { label, value, sub, children, subSnippet } = $props();
</script>

<div class="tile">
  <span class="label">{label}</span>
  {#if children}
    <span class="value">{@render children()}</span>
  {:else}
    <span class="value">{value}</span>
  {/if}
  {#if subSnippet}
    <span class="sub">{@render subSnippet()}</span>
  {:else if sub}
    <span class="sub">{sub}</span>
  {/if}
</div>
