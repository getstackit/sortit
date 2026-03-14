import { PersonDetailPage } from "@/components/person-detail-page";

export default async function PersonPage({
  params,
}: {
  params: Promise<{ person: string }>;
}) {
  const { person } = await params;

  return <PersonDetailPage person={decodeURIComponent(person)} />;
}
