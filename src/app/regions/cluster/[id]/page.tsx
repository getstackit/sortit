"use client";

import { use } from "react";

import { ClusterRegionDetailPage } from "@/components/cluster-region-detail-page";

export default function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  return <ClusterRegionDetailPage id={id} />;
}
