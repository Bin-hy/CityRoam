import { Route, Routes } from "react-router-dom";
import { CreatePage } from "@/features/create/CreatePage";
import { EvalDashboard } from "@/features/eval/EvalDashboard";
import { NotFoundPage } from "@/pages/NotFoundPage";
import { SharePage } from "@/features/share/SharePage";
import { WorkspacePage } from "@/features/workspace/WorkspacePage";

export function App() {
  return (
    <Routes>
      <Route path="/" element={<CreatePage />} />
      <Route path="/trips/:tripId" element={<WorkspacePage />} />
      <Route path="/share/:slug" element={<SharePage />} />
      <Route path="/eval" element={<EvalDashboard />} />
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}
