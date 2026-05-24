"use client";

export function BackgroundMesh() {
  return (
    <div
      className="pointer-events-none fixed inset-0 -z-10 overflow-hidden"
      aria-hidden="true"
    >
      <div
        className="absolute -left-1/4 -top-1/4 h-[50vw] w-[50vw] rounded-full opacity-20 blur-[120px]"
        style={{
          background: "var(--gradient-start)",
          animation: "mesh-float 20s ease-in-out infinite",
        }}
      />
      <div
        className="absolute -right-1/4 top-1/3 h-[40vw] w-[40vw] rounded-full opacity-15 blur-[120px]"
        style={{
          background: "var(--gradient-mid)",
          animation: "mesh-float 25s ease-in-out infinite 5s",
        }}
      />
      <div
        className="absolute -bottom-1/4 left-1/3 h-[45vw] w-[45vw] rounded-full opacity-10 blur-[120px]"
        style={{
          background: "var(--gradient-end)",
          animation: "mesh-float 22s ease-in-out infinite 10s",
        }}
      />
    </div>
  );
}
