import { IssuesBoard } from "@/components/issues-board";
import { INITIAL_ISSUES } from "@/lib/sample-issues";

export default function Home() {
  return <IssuesBoard initialIssues={INITIAL_ISSUES} />;
}
