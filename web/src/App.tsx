import { lazy, Suspense } from "react";
import { Route, Routes } from "react-router-dom";

const CreatePage = lazy(() => import("@/features/create/CreatePage").then((module) => ({ default: module.CreatePage })));
const WorkspacePage = lazy(() =>
  import("@/features/workspace/WorkspacePage").then((module) => ({ default: module.WorkspacePage })),
);
const SharePage = lazy(() => import("@/features/share/SharePage").then((module) => ({ default: module.SharePage })));
const EvalDashboard = lazy(() =>
  import("@/features/eval/EvalDashboard").then((module) => ({ default: module.EvalDashboard })),
);
const NotFoundPage = lazy(() => import("@/pages/NotFoundPage").then((module) => ({ default: module.NotFoundPage })));

export function App() {
  return (
    <Suspense
      fallback={
        <main className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">加载页面…</main>
      }
    >
      <Routes>
        <Route path="/" element={<CreatePage />} />
        <Route path="/trips/:tripId" element={<WorkspacePage />} />
        <Route path="/share/:slug" element={<SharePage />} />
        <Route path="/eval" element={<EvalDashboard />} />
        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </Suspense>
  );
}
