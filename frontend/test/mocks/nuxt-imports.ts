type TestFactory<T> = (() => T) | undefined;

function readFactory<T>(key: string): TestFactory<T> {
  const candidate = (globalThis as Record<string, unknown>)[key];
  return typeof candidate === "function" ? (candidate as () => T) : undefined;
}

export function useNuxtApp() {
  const factory = readFactory<unknown>("__useNuxtAppMock");
  if (!factory) {
    throw new Error("useNuxtApp mock is not configured");
  }
  return factory();
}

export function useRuntimeConfig() {
  const factory = readFactory<unknown>("__useRuntimeConfigMock");
  return factory ? factory() : { public: {} };
}

export function useRequestHeaders() {
  const factory = readFactory<Record<string, string>>("__useRequestHeadersMock");
  return factory ? factory() : {};
}

export function useRouter() {
  const factory = readFactory<unknown>("__useRouterMock");
  if (!factory) {
    throw new Error("useRouter mock is not configured");
  }
  return factory();
}

export function navigateTo(...args: unknown[]) {
  const factory = readFactory<(...callArgs: unknown[]) => unknown>("__navigateToMock");
  if (!factory) {
    throw new Error("navigateTo mock is not configured");
  }
  return factory()(...args);
}
