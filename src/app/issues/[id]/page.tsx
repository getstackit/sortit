import { IssueDetailPage } from "@/components/issue-detail-page";

export default async function IssuePage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const resolvedParams = await params;

  return <IssueDetailPage issueID={resolvedParams.id} />;
}
