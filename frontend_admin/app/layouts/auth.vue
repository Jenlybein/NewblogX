<template>
  <!-- 登录 / 初始化页没有 AdminShell 顶栏，开关单独浮动在右上角：
       「每个页面都能切」是硬要求，而这两页恰好是进站第一眼。 -->
  <div class="admin-auth-shell">
    <div class="admin-auth-theme-toggle">
      <AdminThemeToggle />
    </div>
    <slot />
  </div>
</template>
