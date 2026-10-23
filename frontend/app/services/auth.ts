import type { EmailVerifyPayload } from "~/types/api";

export function loginWithPassword(payload: { username: string; password: string }) {
  return useNuxtApp().$api.request<string>("/api/users/login", {
    method: "POST",
    body: payload,
    auth: false,
    retryAuth: false,
  });
}

export function sendEmailCode(payload: { email: string; type: 1 | 2 | 3 | 4 }) {
  return useNuxtApp().$api.request<EmailVerifyPayload>("/api/users/email/verify", {
    method: "POST",
    body: payload,
    auth: false,
    retryAuth: false,
  });
}

export function loginWithEmailCode(payload: { email_id: string; email_code: string }) {
  return useNuxtApp().$api.request<string>("/api/users/email/login", {
    method: "POST",
    body: payload,
    auth: false,
    retryAuth: false,
  });
}

export function registerWithEmail(payload: { pwd: string; email_id: string; email_code: string }) {
  return useNuxtApp().$api.request<string>("/api/users/email/register", {
    method: "POST",
    body: payload,
    auth: false,
    retryAuth: false,
  });
}

export function logoutCurrentSession() {
  return useNuxtApp().$api.request("/api/users/logout", {
    method: "POST",
    auth: true,
    retryAuth: false,
  });
}
