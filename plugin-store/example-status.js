// KANO_PLUGIN_START
// name: Example Status Plugin
// version: 0.1.0
<script>
(() => {
  const marker = document.createElement('small');
  marker.textContent = 'Open self-hosted plugin catalog active';
  marker.dataset.ufiPlugin = 'example-status';
  document.body.appendChild(marker);
})();
</script>
// KANO_PLUGIN_END

