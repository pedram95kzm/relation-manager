import React from "react";
import ReactDOM from "react-dom/client";
import {
  createBrowserRouter,
  createRoutesFromElements,
  RouterProvider,
  Navigate,
  Outlet,
  Route,
  Link,
  NavLink,
} from "react-router-dom";
import {
  Compass,
  LayoutDashboard,
  BookOpen,
  Settings,
  ShieldCheck,
  LogOut,
  ArrowUpRight,
} from "lucide-react";
import { Preferences, usePreferences } from "./i18n";
import { AuthProvider, useAuth } from "./auth";
import { logout } from "./api";
import { ErrorBox, Loading, PreferencesButtons } from "./components/ui";
import { AuthPage } from "./pages/AuthPage";
import { Dashboard, History } from "./pages/Dashboard";
import { Account } from "./pages/Account";
import { Wizard } from "./pages/Wizard";
import { Results } from "./pages/Results";
import { Admin } from "./pages/Admin";
import "./style.css";
import "@fontsource-variable/manrope";
import "@fontsource-variable/vazirmatn";

function Shell() {
  const { user, loading, error } = useAuth(),
    { t } = usePreferences();
  if (loading) return <Loading />;
  if (!user) return <Navigate to="/login" replace />;
  return (
    <div className="app-shell">
      <a className="skip-link" href="#content">
        {t("next")}
      </a>
      <aside className="sidebar">
        <Link className="brand" to="/">
          <span className="brand-mark">
            <Compass size={23} />
          </span>
          relations<span className="brand-dot">.</span>
        </Link>
        <p className="sidebar-label">{t("privacy")}</p>
        <nav aria-label={t("dashboard")}>
          <NavLink to="/" end>
            <LayoutDashboard size={19} />
            {t("dashboard")}
          </NavLink>
          <NavLink to="/history">
            <BookOpen size={19} />
            {t("history")}
          </NavLink>
          <NavLink to="/account">
            <Settings size={19} />
            {t("settings")}
          </NavLink>
          {user.role === "admin" && (
            <NavLink to="/admin">
              <ShieldCheck size={19} />
              {t("admin")}
            </NavLink>
          )}
        </nav>
        <div className="sidebar-note">
          <div className="tiny-orbit" />
          <strong>{t("privacy")}</strong>
          <p>{t("privacyText")}</p>
        </div>
        <div className="account-chip">
          <span className="avatar">{user.name.slice(0, 1).toUpperCase()}</span>
          <div>
            <strong>{user.name}</strong>
            <small>{user.email}</small>
          </div>
        </div>
        <button
          className="signout"
          onClick={() => logout().catch((e) => window.alert(e.message))}
        >
          <LogOut size={17} />
          {t("signOut")}
        </button>
      </aside>
      <div className="workspace">
        <header className="topbar">
          <span className="topbar-label">
            {t("dashboard")} <ArrowUpRight size={14} />
          </span>
          <div className="header-actions">
            <PreferencesButtons />
            <button
              className="icon-button mobile-signout"
              aria-label={t("signOut")}
              onClick={() => logout().catch((e) => window.alert(e.message))}
            >
              <LogOut size={18} />
            </button>
          </div>
        </header>
        <main id="content" tabIndex={-1}>
          <ErrorBox message={error} />
          <Outlet />
        </main>
        <footer>{t("disclaimer")}</footer>
      </div>
    </div>
  );
}
function NotFound() {
  const { t } = usePreferences();
  return (
    <div className="empty">
      <h1>{t("notFound")}</h1>
      <Link to="/">{t("home")}</Link>
    </div>
  );
}
function Root() {
  return (
    <AuthProvider>
      <Outlet />
    </AuthProvider>
  );
}
const router = createBrowserRouter(
  createRoutesFromElements(
    <Route element={<Root />}>
      <Route path="/login" element={<AuthPage />} />
      <Route path="/register" element={<AuthPage register />} />
      <Route path="/auth/callback" element={<Navigate to="/" replace />} />
      <Route element={<Shell />}>
        <Route index element={<Dashboard />} />
        <Route path="history" element={<History />} />
        <Route path="account" element={<Account />} />
        <Route path="reflect/:module" element={<Wizard />} />
        <Route path="analyses/:id/edit" element={<Wizard />} />
        <Route path="analyses/:id" element={<Results />} />
        <Route path="admin" element={<Admin />} />
      </Route>
      <Route path="*" element={<NotFound />} />
    </Route>,
  ),
);
function App() {
  return (
    <Preferences>
      <RouterProvider router={router} />
    </Preferences>
  );
}
ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
