<template>
  <main class="shell">
    <nav class="topbar">
      <router-link to="/feed">Feed</router-link>
      <router-link to="/publish">发布</router-link>
      <router-link to="/login">登录</router-link>
    </nav>
    <router-view />
  </main>
</template>
