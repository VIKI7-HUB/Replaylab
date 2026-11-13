import React, { Suspense, lazy } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import { AppShell } from './AppShell';
import { Skeleton } from '../components/States';
import { PRODUCT_HOME } from './consoles';

// Lazy-loaded routes as required by §15
const CapturesPage = lazy(() =>
  import('../features/replaylab/CapturesPage').then((m) => ({ default: m.CapturesPage }))
);
const ExperimentsListPage = lazy(() =>
  import('../features/replaylab/ExperimentsListPage').then((m) => ({
    default: m.ExperimentsListPage,
  }))
);
const NewExperimentPage = lazy(() =>
  import('../features/replaylab/NewExperimentPage').then((m) => ({
    default: m.NewExperimentPage,
  }))
);
const ExperimentDetailPage = lazy(() =>
  import('../features/replaylab/ExperimentDetailPage').then((m) => ({
    default: m.ExperimentDetailPage,
  }))
);
const SettingsPage = lazy(() =>
  import('../features/replaylab/SettingsPage').then((m) => ({ default: m.SettingsPage }))
);

const CheckPage = lazy(() =>
  import('../features/stale/CheckPage').then((m) => ({ default: m.CheckPage }))
);
const ReportPage = lazy(() =>
  import('../features/stale/ReportPage').then((m) => ({ default: m.ReportPage }))
);
const HistoryPage = lazy(() =>
  import('../features/stale/HistoryPage').then((m) => ({ default: m.HistoryPage }))
);

const PageFallback = () => (
  <div className="space-y-6">
    <Skeleton height="h-8" width="w-48" />
    <Skeleton height="h-4" width="w-96" />
    <Skeleton height="h-64" />
  </div>
);

export const AppRoutes: React.FC = () => {
  return (
    <Routes>
      <Route path="/" element={<AppShell />}>
        <Route index element={<Navigate to={PRODUCT_HOME.replaylab} replace />} />

        {/* Replay Lab routes */}
        <Route
          path="replaylab/captures"
          element={
            <Suspense fallback={<PageFallback />}>
              <CapturesPage />
            </Suspense>
          }
        />
        <Route
          path="replaylab/captures/:id"
          element={
            <Suspense fallback={<PageFallback />}>
              <CapturesPage />
            </Suspense>
          }
        />
        <Route
          path="replaylab/experiments"
          element={
            <Suspense fallback={<PageFallback />}>
              <ExperimentsListPage />
            </Suspense>
          }
        />
        <Route
          path="replaylab/experiments/new"
          element={
            <Suspense fallback={<PageFallback />}>
              <NewExperimentPage />
            </Suspense>
          }
        />
        <Route
          path="replaylab/experiments/:id"
          element={
            <Suspense fallback={<PageFallback />}>
              <ExperimentDetailPage />
            </Suspense>
          }
        />
        <Route
          path="replaylab/settings"
          element={
            <Suspense fallback={<PageFallback />}>
              <SettingsPage />
            </Suspense>
          }
        />

        {/* Stale answers routes */}
        <Route
          path="stale/check"
          element={
            <Suspense fallback={<PageFallback />}>
              <CheckPage />
            </Suspense>
          }
        />
        <Route
          path="stale/reports/:id"
          element={
            <Suspense fallback={<PageFallback />}>
              <ReportPage />
            </Suspense>
          }
        />
        <Route
          path="stale/history"
          element={
            <Suspense fallback={<PageFallback />}>
              <HistoryPage />
            </Suspense>
          }
        />

        {/* Catch-all redirect */}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
};
