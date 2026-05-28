"use client";

import { use } from "react";

import { CustomRegionDetailPage } from "@/components/custom-region-detail-page";

export default function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  return <CustomRegionDetailPage id={id} />;
}
