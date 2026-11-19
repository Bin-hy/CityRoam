import { Route, Routes } from "react-router-dom";
import { CreatePage } from "@/pages/CreatePage";
import { EvalPage } from "@/pages/EvalPage";
import { NotFoundPage } from "@/pages/NotFoundPage";
import { SharePage } from "@/pages/SharePage";
import { WorkspacePage } from "@/pages/WorkspacePage";

export function App() {
  return (
    <Routes>
      <Route path="/" element={<CreatePage />} />
      <Route path="/trips/:tripId" element={<WorkspacePage />} />
      <Route path="/share/:slug" element={<SharePage />} />
      <Route path="/eval" element={<EvalPage />} />
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}
