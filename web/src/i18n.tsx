/* eslint-disable react-refresh/only-export-components -- this module intentionally
   exports the provider component alongside the useI18n hook and locale data. */
import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";

// Lightweight, dependency-free i18n (the project keeps its dependency list
// tight). Strings live in the dictionaries below keyed by dot-path; t() looks
// up the active language, falls back to English, then to the key itself, and
// interpolates {{vars}}. The choice is remembered per browser.

export type Lang = "en" | "pl" | "de" | "es" | "fr";

export const LANGUAGES: { id: Lang; label: string }[] = [
  { id: "en", label: "English" },
  { id: "pl", label: "Polski" },
  { id: "de", label: "Deutsch" },
  { id: "es", label: "Español" },
  { id: "fr", label: "Français" },
];

type Dict = Record<string, string>;

const en: Dict = {
  "nav.home": "Home",
  "nav.stacks": "Stacks",
  "nav.updates": "Updates",
  "nav.settings": "Settings",
  "sidebar.backendOk": "Backend ok",
  "sidebar.backendDown": "Backend unreachable",
  "sidebar.signOut": "Sign out",
  "settings.title": "Settings",
  "settings.appearance": "Appearance",
  "settings.appearanceHelp":
    "The theme is saved in this browser only. It isn't shared with other users or devices.",
  "settings.language": "Language",
  "settings.languageHelp": "The interface language, saved in this browser.",
  "settings.autoUpdate": "Automatic update check",
  "settings.selfUpdate": "HiveDock self-update",
  "settings.versionHistory": "Version history",
  "settings.apiToken": "Read-only API token",
  "settings.registries": "Private registries",
  "settings.maintenance": "Maintenance",
  "settings.environment": "Environment",
  "common.save": "Save",
  "common.saving": "Saving…",
};

const pl: Dict = {
  "nav.home": "Start",
  "nav.stacks": "Stacks",
  "nav.updates": "Aktualizacje",
  "nav.settings": "Ustawienia",
  "sidebar.backendOk": "Serwer OK",
  "sidebar.backendDown": "Serwer niedostępny",
  "sidebar.signOut": "Wyloguj",
  "settings.title": "Ustawienia",
  "settings.appearance": "Wygląd",
  "settings.appearanceHelp":
    "Motyw jest zapisywany tylko w tej przeglądarce. Nie jest współdzielony z innymi użytkownikami ani urządzeniami.",
  "settings.language": "Język",
  "settings.languageHelp": "Język interfejsu, zapisywany w tej przeglądarce.",
  "settings.autoUpdate": "Automatyczne sprawdzanie aktualizacji",
  "settings.selfUpdate": "Samoaktualizacja HiveDock",
  "settings.versionHistory": "Historia wersji",
  "settings.apiToken": "Token API tylko do odczytu",
  "settings.registries": "Prywatne rejestry",
  "settings.maintenance": "Konserwacja",
  "settings.environment": "Środowisko",
  "common.save": "Zapisz",
  "common.saving": "Zapisywanie…",
};

const de: Dict = {
  "nav.home": "Start",
  "nav.stacks": "Stacks",
  "nav.updates": "Updates",
  "nav.settings": "Einstellungen",
  "sidebar.backendOk": "Backend OK",
  "sidebar.backendDown": "Backend nicht erreichbar",
  "sidebar.signOut": "Abmelden",
  "settings.title": "Einstellungen",
  "settings.appearance": "Darstellung",
  "settings.appearanceHelp":
    "Das Thema wird nur in diesem Browser gespeichert. Es wird nicht mit anderen Benutzern oder Geräten geteilt.",
  "settings.language": "Sprache",
  "settings.languageHelp": "Die Sprache der Oberfläche, in diesem Browser gespeichert.",
  "settings.autoUpdate": "Automatische Update-Prüfung",
  "settings.selfUpdate": "HiveDock-Selbstaktualisierung",
  "settings.versionHistory": "Versionsverlauf",
  "settings.apiToken": "Schreibgeschütztes API-Token",
  "settings.registries": "Private Registries",
  "settings.maintenance": "Wartung",
  "settings.environment": "Umgebung",
  "common.save": "Speichern",
  "common.saving": "Speichern…",
};

const es: Dict = {
  "nav.home": "Inicio",
  "nav.stacks": "Stacks",
  "nav.updates": "Actualizaciones",
  "nav.settings": "Ajustes",
  "sidebar.backendOk": "Backend OK",
  "sidebar.backendDown": "Backend no disponible",
  "sidebar.signOut": "Cerrar sesión",
  "settings.title": "Ajustes",
  "settings.appearance": "Apariencia",
  "settings.appearanceHelp":
    "El tema se guarda solo en este navegador. No se comparte con otros usuarios ni dispositivos.",
  "settings.language": "Idioma",
  "settings.languageHelp": "El idioma de la interfaz, guardado en este navegador.",
  "settings.autoUpdate": "Comprobación automática de actualizaciones",
  "settings.selfUpdate": "Autoactualización de HiveDock",
  "settings.versionHistory": "Historial de versiones",
  "settings.apiToken": "Token de API de solo lectura",
  "settings.registries": "Registros privados",
  "settings.maintenance": "Mantenimiento",
  "settings.environment": "Entorno",
  "common.save": "Guardar",
  "common.saving": "Guardando…",
};

const fr: Dict = {
  "nav.home": "Accueil",
  "nav.stacks": "Stacks",
  "nav.updates": "Mises à jour",
  "nav.settings": "Paramètres",
  "sidebar.backendOk": "Backend OK",
  "sidebar.backendDown": "Backend injoignable",
  "sidebar.signOut": "Déconnexion",
  "settings.title": "Paramètres",
  "settings.appearance": "Apparence",
  "settings.appearanceHelp":
    "Le thème est enregistré uniquement dans ce navigateur. Il n'est pas partagé avec d'autres utilisateurs ou appareils.",
  "settings.language": "Langue",
  "settings.languageHelp": "La langue de l'interface, enregistrée dans ce navigateur.",
  "settings.autoUpdate": "Vérification automatique des mises à jour",
  "settings.selfUpdate": "Mise à jour automatique de HiveDock",
  "settings.versionHistory": "Historique des versions",
  "settings.apiToken": "Jeton d'API en lecture seule",
  "settings.registries": "Registres privés",
  "settings.maintenance": "Maintenance",
  "settings.environment": "Environnement",
  "common.save": "Enregistrer",
  "common.saving": "Enregistrement…",
};

const dicts: Record<Lang, Dict> = { en, pl, de, es, fr };
const STORAGE_KEY = "hivedock-lang";

function interpolate(s: string, vars?: Record<string, string | number>): string {
  if (!vars) return s;
  return s.replace(/\{\{(\w+)\}\}/g, (_, k) =>
    vars[k] !== undefined ? String(vars[k]) : `{{${k}}}`,
  );
}

export type TFunc = (key: string, vars?: Record<string, string | number>) => string;

function translate(lang: Lang, key: string, vars?: Record<string, string | number>) {
  return interpolate(dicts[lang][key] ?? dicts.en[key] ?? key, vars);
}

type Ctx = { lang: Lang; setLang: (l: Lang) => void; t: TFunc };

// Default context resolves English, so components (and tests) work even without
// a provider; the real app wraps everything in I18nProvider (see main.tsx).
const I18nContext = createContext<Ctx>({
  lang: "en",
  setLang: () => {},
  t: (key, vars) => translate("en", key, vars),
});

function detect(): Lang {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored && stored in dicts) return stored as Lang;
    const nav = navigator.language.slice(0, 2);
    if (nav in dicts) return nav as Lang;
  } catch {
    /* localStorage / navigator unavailable */
  }
  return "en";
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(detect);
  useEffect(() => {
    document.documentElement.lang = lang;
  }, [lang]);
  const setLang = (l: Lang) => {
    setLangState(l);
    try {
      localStorage.setItem(STORAGE_KEY, l);
    } catch {
      /* ignore */
    }
  };
  const t: TFunc = (key, vars) => translate(lang, key, vars);
  return (
    <I18nContext.Provider value={{ lang, setLang, t }}>{children}</I18nContext.Provider>
  );
}

export function useI18n(): Ctx {
  return useContext(I18nContext);
}
