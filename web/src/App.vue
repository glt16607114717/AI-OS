<template>
  <div class="app-wrapper">
    <router-view />
  </div>
</template>

<script setup lang="ts">
</script>

<style>
html,
body,
#app,
.app-wrapper {
  height: 100vh !important;
  width: 100vw !important;
  margin: 0 !important;
  padding: 0 !important;
  overflow: hidden !important;
}

* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: "Segoe UI", -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", sans-serif;
  -webkit-font-smoothing: antialiased;
}
</style>
