const LOCAL_API_ORIGIN = "http://127.0.0.1:8081";

function trimTrailingSlash(value: string) {
  return value.endsWith("/") ? value.slice(0, -1) : value;
}

function explicitAPIOrigin() {
  const configured = process.env.NEXT_PUBLIC_API_ORIGIN?.trim();
  return configured ? trimTrailingSlash(configured) : "";
}

function shouldUseLocalAPIOrigin() {
  if (process.env.NODE_ENV !== "development") {
    return false;
  }

  if (typeof window === "undefined") {
    return false;
  }

  const { hostname } = window.location;
  return hostname === "localhost" || hostname === "127.0.0.1";
}

export function apiURL(path: string) {
  const explicitOrigin = explicitAPIOrigin();
  if (explicitOrigin) {
    return `${explicitOrigin}${path}`;
  }

  if (shouldUseLocalAPIOrigin()) {
    return `${LOCAL_API_ORIGIN}${path}`;
  }

  return path;
}
