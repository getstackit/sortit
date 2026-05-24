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

function localAPIOrigin() {
  if (typeof window === "undefined") {
    return "";
  }

  return `${window.location.protocol}//${window.location.hostname}:8081`;
}

export function apiURL(path: string) {
 const explicitOrigin = explicitAPIOrigin();
 if (explicitOrigin) {
    return `${explicitOrigin}${path}`;
  }

  if (shouldUseLocalAPIOrigin()) {
    return `${localAPIOrigin()}${path}`;
  }

  return path;
}

function joinAPIPath(prefix: string, path: string) {
  if (!path.startsWith("/")) {
    return `${prefix}/${path}`;
  }
  return `${prefix}${path}`;
}

export function uiAPIURL(path: string) {
  return apiURL(joinAPIPath("/api/ui", path));
}

export function versionedAPIURL(path: string) {
  return apiURL(joinAPIPath("/api/v1", path));
}
