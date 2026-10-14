import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import type { Locale } from "./types";

const messages = {
  dashboard: ["Overview", "نمای کلی"],
  history: ["My reflections", "تأمل‌های من"],
  settings: ["Account", "حساب کاربری"],
  admin: ["Administration", "مدیریت"],
  signOut: ["Sign out", "خروج"],
  welcome: [
    "A little clarity, one reflection at a time.",
    "کمی شفافیت، با هر بار تأمل.",
  ],
  welcomeText: [
    "A calm space to understand your patterns, compare perspectives, and approach difficult conversations.",
    "فضایی آرام برای شناخت الگوهای خود، مقایسه دیدگاه‌ها و گفت‌وگوهای دشوار.",
  ],
  self: ["Understand yourself", "شناخت خود"],
  compatibility: ["Explore your connection", "بررسی ارتباط"],
  conflict: ["Work through a conflict", "بررسی یک تعارض"],
  selfText: [
    "Reflect on your personality, emotions, and communication.",
    "درباره شخصیت، احساسات و ارتباط خود تأمل کنید.",
  ],
  compatibilityText: [
    "Compare independent responses with your partner’s consent.",
    "پاسخ‌های مستقل را با رضایت طرف مقابل مقایسه کنید.",
  ],
  conflictText: [
    "Consider the situation and find a thoughtful next step.",
    "موقعیت را بررسی کنید و گام بعدی را آگاهانه انتخاب کنید.",
  ],
  start: ["Begin reflection", "شروع تأمل"],
  recent: ["Recent reflections", "تأمل‌های اخیر"],
  viewAll: ["View all", "مشاهده همه"],
  empty: [
    "Your next insight starts here.",
    "بینش بعدی شما از اینجا آغاز می‌شود.",
  ],
  emptyText: [
    "Start a reflection above. Your drafts and results will appear here.",
    "یک تأمل را شروع کنید. پیش‌نویس‌ها و نتایج شما اینجا نمایش داده می‌شوند.",
  ],
  disclaimer: [
    "For educational reflection. These original questionnaires are not validated clinical tests and do not diagnose or predict relationship success.",
    "برای تأمل آموزشی. این پرسش‌نامه‌های تألیفی آزمون بالینی معتبر نیستند و تشخیص یا پیش‌بینی موفقیت رابطه ارائه نمی‌کنند.",
  ],
  draft: ["Draft", "پیش‌نویس"],
  processing: ["Processing", "در حال پردازش"],
  completed: ["Complete", "تکمیل‌شده"],
  insufficient: ["Limited data", "داده ناکافی"],
  failed: ["AI report failed", "گزارش هوش مصنوعی ناموفق"],
  loading: ["Loading…", "در حال بارگذاری…"],
  retry: ["Try again", "تلاش دوباره"],
  save: ["Save changes", "ذخیره تغییرات"],
  saved: ["Saved", "ذخیره شد"],
  error: ["Something needs your attention", "خطایی رخ داده است"],
  login: ["Sign in", "ورود"],
  register: ["Create account", "ایجاد حساب"],
  email: ["Email address", "نشانی ایمیل"],
  password: ["Password", "رمز عبور"],
  name: ["Your name", "نام شما"],
  google: ["Continue with Google", "ادامه با گوگل"],
  loginTitle: ["Make room for reflection.", "برای تأمل وقت بگذارید."],
  loginText: [
    "Understand yourself. Listen with curiosity. Find your next step.",
    "خود را بشناسید. با کنجکاوی گوش دهید. گام بعدی را پیدا کنید.",
  ],
  passwordHint: [
    "Use 12–72 UTF-8 bytes. A longer unique password is best.",
    "از ۱۲ تا ۷۲ بایت استفاده کنید. رمز منحصربه‌فرد و طولانی بهتر است.",
  ],
  profileRequired: [
    "Complete your profile to begin a reflection.",
    "برای شروع تأمل، نمایه خود را تکمیل کنید.",
  ],
  age: ["Age", "سن"],
  gender: ["Gender", "جنسیت"],
  profile: ["Your profile", "نمایه شما"],
  female: ["Female", "زن"],
  male: ["Male", "مرد"],
  nonbinary: ["Nonbinary", "غیردودویی"],
  other: ["Other", "دیگر"],
  prefer_not_to_say: ["Prefer not to say", "ترجیح می‌دهم نگویم"],
  next: ["Continue", "ادامه"],
  back: ["Back", "بازگشت"],
  review: ["Review & submit", "بازبینی و ارسال"],
  saveExit: ["Save & exit", "ذخیره و خروج"],
  selfRespondent: ["Your responses", "پاسخ‌های شما"],
  partnerRespondent: ["Partner’s responses", "پاسخ‌های طرف مقابل"],
  context: ["A little context", "کمی زمینه"],
  answerHint: [
    "There are no right answers. Choose what fits, or “I don’t know”.",
    "پاسخ درست یا غلطی وجود ندارد. گزینه مناسب یا «نمی‌دانم» را انتخاب کنید.",
  ],
  answered: ["Answered", "پاسخ داده‌شده"],
  unknown: ["Unknown", "نامشخص"],
  omitted: ["Unanswered", "بی‌پاسخ"],
  reviewHint: [
    "Review the responses below. Select a question to edit it.",
    "پاسخ‌ها را بازبینی کنید. برای ویرایش یک پرسش، آن را انتخاب کنید.",
  ],
  consent: [
    "I understand the educational limitations and consent to saving and processing these responses.",
    "محدودیت‌های آموزشی را می‌پذیرم و با ذخیره و پردازش پاسخ‌ها موافقم.",
  ],
  aiConsent: [
    "Generate an AI narrative. I consent to sharing these responses and context with the configured AI provider.",
    "گزارش هوش مصنوعی تولید شود. با ارسال پاسخ‌ها و زمینه به ارائه‌دهنده تنظیم‌شده موافقم.",
  ],
  submit: ["See my results", "مشاهده نتایج"],
  submitting: ["Preparing your reflection…", "در حال آماده‌سازی تأمل شما…"],
  results: ["Your reflection", "تأمل شما"],
  scores: ["Response patterns", "الگوهای پاسخ"],
  scoreHint: [
    "Scores describe your answers on a 0–100 scale. Separate dimensions do not add up to 100. A dash means too little numeric data.",
    "امتیازها پاسخ‌های شما را در مقیاس صفر تا صد توصیف می‌کنند. ابعاد جداگانه جمعاً صد نمی‌شوند. خط تیره یعنی داده عددی ناکافی.",
  ],
  dimension: ["Dimension", "بُعد"],
  score: ["Score", "امتیاز"],
  completeness: ["Numeric completeness", "کامل‌بودن عددی"],
  limitations: ["Interpret with care", "با دقت تفسیر کنید"],
  similarity: ["Answer similarity", "شباهت پاسخ‌ها"],
  similarityHint: [
    "Descriptive similarity, not a probability of relationship success.",
    "شباهت توصیفی، نه احتمال موفقیت رابطه.",
  ],
  export: ["Print / save PDF", "چاپ / ذخیره PDF"],
  delete: ["Delete", "حذف"],
  deleteConfirm: [
    "Delete this reflection permanently?",
    "این تأمل برای همیشه حذف شود؟",
  ],
  all: ["All", "همه"],
  filter: ["Filter", "فیلتر"],
  previous: ["Previous", "قبلی"],
  page: ["Page", "صفحه"],
  noData: ["No records yet.", "هنوز رکوردی وجود ندارد."],
  security: ["Password & security", "رمز عبور و امنیت"],
  currentPassword: ["Current password", "رمز فعلی"],
  newPassword: ["New password", "رمز جدید"],
  changePassword: ["Change password", "تغییر رمز"],
  logoutAll: ["Sign out of all sessions", "خروج از همه نشست‌ها"],
  deleteAccount: ["Delete my account", "حذف حساب من"],
  deleteAccountHint: [
    "Permanently erase your account, drafts and reports. Password accounts only; administrators cannot delete themselves.",
    "حساب، پیش‌نویس‌ها و گزارش‌ها برای همیشه حذف می‌شوند. فقط حساب دارای رمز؛ مدیر نمی‌تواند خود را حذف کند.",
  ],
  provider: ["AI provider", "ارائه‌دهنده هوش مصنوعی"],
  users: ["Users", "کاربران"],
  limits: ["Daily limits", "محدودیت روزانه"],
  audit: ["Audit log", "گزارش ممیزی"],
  analytics: ["Activity", "فعالیت"],
  model: ["Model ID", "شناسه مدل"],
  apiKey: ["New API key", "کلید API جدید"],
  active: ["Enable AI generation", "فعال‌سازی هوش مصنوعی"],
  test: ["Test connection", "آزمایش اتصال"],
  globalLimit: ["Global daily limit", "محدودیت روزانه عمومی"],
  userLimit: ["Default user daily limit", "محدودیت پیش‌فرض کاربر"],
  disable: ["Disable", "غیرفعال"],
  enable: ["Enable", "فعال"],
  search: ["Search by name or email", "جست‌وجوی نام یا ایمیل"],
  quota: ["AI reports today", "گزارش‌های هوش مصنوعی امروز"],
  privacy: ["Your responses belong to you", "پاسخ‌های شما متعلق به شماست"],
  privacyText: [
    "Drafts stay in your account. AI sharing is optional and requires your consent each time.",
    "پیش‌نویس‌ها در حساب شما می‌مانند. ارسال به هوش مصنوعی اختیاری است و هر بار رضایت شما لازم است.",
  ],
  notFound: ["This page could not be found.", "این صفحه پیدا نشد."],
  home: ["Return to overview", "بازگشت به نمای کلی"],
  unsaved: [
    "You have unsaved responses. Leave this page?",
    "پاسخ‌های ذخیره‌نشده دارید. از صفحه خارج می‌شوید؟",
  ],
  questions: ["Questions", "پرسش‌ها"],
  confirmEmail: ["Confirm account email", "تأیید ایمیل حساب"],
  cancel: ["Cancel", "لغو"],
  eraseUser: [
    "Erase this ordinary account and all its saved reflections permanently. Enter your administrator password and the exact account email.",
    "این حساب عادی و تمام تأمل‌های ذخیره‌شده آن برای همیشه حذف می‌شوند. رمز مدیر و ایمیل دقیق حساب را وارد کنید.",
  ],
  localExample: ["Local test provider", "ارائه‌دهنده آزمایشی محلی"],
  localExampleReport: ["Local example report", "گزارش نمونه محلی"],
  localExampleInfo: [
    "This example is generated locally. No prompt or response is sent to an external AI. It demonstrates report structure rather than personalized AI insight.",
    "این نمونه به‌صورت محلی تولید می‌شود و هیچ پرسش یا پاسخی به هوش مصنوعی خارجی ارسال نمی‌شود. این متن، ساختار گزارش را نشان می‌دهد و بینش شخصی هوش مصنوعی نیست.",
  ],
  localExampleChoice: [
    "Generate a local example report without sending my responses externally.",
    "گزارش نمونه محلی بدون ارسال پاسخ‌های من به سرویس خارجی تولید شود.",
  ],
  previewExample: ["Preview example response", "پیش‌نمایش پاسخ نمونه"],
  responseJson: ["Example API response", "پاسخ نمونه API"],
  maxTokens: ["Maximum output tokens", "حداکثر توکن خروجی"],
  timeoutSeconds: ["Timeout (seconds)", "مهلت (ثانیه)"],
  retries: ["Retries", "تلاش مجدد"],
} as const;
export type MessageKey = keyof typeof messages;
const Context = createContext<{
  locale: Locale;
  setLocale: (v: Locale) => void;
  theme: string;
  toggleTheme: () => void;
  t: (key: MessageKey) => string;
}>(null!);
function preference(key: string, fallback: string) {
  try {
    return localStorage.getItem(key) || fallback;
  } catch {
    return fallback;
  }
}
export function Preferences({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState<Locale>(
    preference("relations-locale", "fa") === "en" ? "en" : "fa",
  );
  const [theme, setTheme] = useState(
    preference(
      "relations-theme",
      window.matchMedia?.("(prefers-color-scheme: dark)").matches
        ? "dark"
        : "light",
    ),
  );
  useEffect(() => {
    document.documentElement.lang = locale;
    document.documentElement.dir = locale === "fa" ? "rtl" : "ltr";
    try {
      localStorage.setItem("relations-locale", locale);
    } catch {}
  }, [locale]);
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem("relations-theme", theme);
    } catch {}
  }, [theme]);
  return (
    <Context.Provider
      value={{
        locale,
        setLocale,
        theme,
        toggleTheme: () => setTheme(theme === "light" ? "dark" : "light"),
        t: (key) => messages[key][locale === "en" ? 0 : 1],
      }}
    >
      {children}
    </Context.Provider>
  );
}
export const usePreferences = () => useContext(Context);
