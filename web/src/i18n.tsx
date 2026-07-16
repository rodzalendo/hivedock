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
  "common.cancel": "Cancel",
  "settings.env.stacksDir": "Stacks directory",
  "settings.env.dataDir": "Data directory",
  "settings.env.publicHost": "Public host",
  "settings.env.auth": "Authentication",
  "settings.env.version": "Version",
  "dashboard.search": "Search apps…",
  "dashboard.showHidden": "Show hidden ({{n}})",
  "dashboard.active": "active",
  "dashboard.inactive": "inactive",
  "dashboard.exited": "exited",
  "dashboard.customize": "Customize",
  "stacks.new": "New",
  "stacks.deploy": "Deploy",
  "stacks.pull": "Pull",
  "stacks.restart": "Restart",
  "stacks.stop": "Stop",
  "stacks.rename": "Rename",
  "stacks.delete": "Delete",
  "stacks.validate": "Validate",
  "stacks.revert": "Revert",
  "stacks.logs": "Logs",
  "stacks.follow": "Follow",
  "stacks.enlarge": "Enlarge",
  "updates.checkNow": "Check now",
  "updates.checking": "Checking…",
  "updates.allUpToDate": "Everything up to date",
  "updates.ignore": "Ignore",
  "updates.unignore": "Un-ignore",
  "updates.selectAll": "Select all",
  "updates.sectionAvailable": "Update available",
  "updates.sectionIgnored": "Ignored",
  "updates.sectionUpToDate": "Up to date",
  "updates.sectionOther": "Not checked / other",
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
  "common.cancel": "Anuluj",
  "settings.env.stacksDir": "Katalog stacków",
  "settings.env.dataDir": "Katalog danych",
  "settings.env.publicHost": "Host publiczny",
  "settings.env.auth": "Uwierzytelnianie",
  "settings.env.version": "Wersja",
  "dashboard.search": "Szukaj aplikacji…",
  "dashboard.showHidden": "Pokaż ukryte ({{n}})",
  "dashboard.active": "aktywne",
  "dashboard.inactive": "nieaktywne",
  "dashboard.exited": "zakończone",
  "dashboard.customize": "Dostosuj",
  "stacks.new": "Nowy",
  "stacks.deploy": "Wdróż",
  "stacks.pull": "Pobierz",
  "stacks.restart": "Uruchom ponownie",
  "stacks.stop": "Zatrzymaj",
  "stacks.rename": "Zmień nazwę",
  "stacks.delete": "Usuń",
  "stacks.validate": "Sprawdź",
  "stacks.revert": "Przywróć",
  "stacks.logs": "Logi",
  "stacks.follow": "Śledź",
  "stacks.enlarge": "Powiększ",
  "updates.checkNow": "Sprawdź teraz",
  "updates.checking": "Sprawdzanie…",
  "updates.allUpToDate": "Wszystko aktualne",
  "updates.ignore": "Ignoruj",
  "updates.unignore": "Przywróć",
  "updates.selectAll": "Zaznacz wszystko",
  "updates.sectionAvailable": "Dostępna aktualizacja",
  "updates.sectionIgnored": "Ignorowane",
  "updates.sectionUpToDate": "Aktualne",
  "updates.sectionOther": "Niesprawdzone / inne",
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
  "common.cancel": "Abbrechen",
  "settings.env.stacksDir": "Stacks-Verzeichnis",
  "settings.env.dataDir": "Datenverzeichnis",
  "settings.env.publicHost": "Öffentlicher Host",
  "settings.env.auth": "Authentifizierung",
  "settings.env.version": "Version",
  "dashboard.search": "Apps suchen…",
  "dashboard.showHidden": "Ausgeblendete anzeigen ({{n}})",
  "dashboard.active": "aktiv",
  "dashboard.inactive": "inaktiv",
  "dashboard.exited": "beendet",
  "dashboard.customize": "Anpassen",
  "stacks.new": "Neu",
  "stacks.deploy": "Bereitstellen",
  "stacks.pull": "Pull",
  "stacks.restart": "Neu starten",
  "stacks.stop": "Stoppen",
  "stacks.rename": "Umbenennen",
  "stacks.delete": "Löschen",
  "stacks.validate": "Prüfen",
  "stacks.revert": "Zurücksetzen",
  "stacks.logs": "Logs",
  "stacks.follow": "Folgen",
  "stacks.enlarge": "Vergrößern",
  "updates.checkNow": "Jetzt prüfen",
  "updates.checking": "Prüfen…",
  "updates.allUpToDate": "Alles aktuell",
  "updates.ignore": "Ignorieren",
  "updates.unignore": "Nicht mehr ignorieren",
  "updates.selectAll": "Alle auswählen",
  "updates.sectionAvailable": "Update verfügbar",
  "updates.sectionIgnored": "Ignoriert",
  "updates.sectionUpToDate": "Aktuell",
  "updates.sectionOther": "Nicht geprüft / sonstige",
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
  "common.cancel": "Cancelar",
  "settings.env.stacksDir": "Directorio de stacks",
  "settings.env.dataDir": "Directorio de datos",
  "settings.env.publicHost": "Host público",
  "settings.env.auth": "Autenticación",
  "settings.env.version": "Versión",
  "dashboard.search": "Buscar apps…",
  "dashboard.showHidden": "Mostrar ocultos ({{n}})",
  "dashboard.active": "activos",
  "dashboard.inactive": "inactivos",
  "dashboard.exited": "detenidos",
  "dashboard.customize": "Personalizar",
  "stacks.new": "Nuevo",
  "stacks.deploy": "Desplegar",
  "stacks.pull": "Pull",
  "stacks.restart": "Reiniciar",
  "stacks.stop": "Detener",
  "stacks.rename": "Renombrar",
  "stacks.delete": "Eliminar",
  "stacks.validate": "Validar",
  "stacks.revert": "Revertir",
  "stacks.logs": "Registros",
  "stacks.follow": "Seguir",
  "stacks.enlarge": "Ampliar",
  "updates.checkNow": "Comprobar ahora",
  "updates.checking": "Comprobando…",
  "updates.allUpToDate": "Todo actualizado",
  "updates.ignore": "Ignorar",
  "updates.unignore": "No ignorar",
  "updates.selectAll": "Seleccionar todo",
  "updates.sectionAvailable": "Actualización disponible",
  "updates.sectionIgnored": "Ignorados",
  "updates.sectionUpToDate": "Actualizado",
  "updates.sectionOther": "Sin comprobar / otros",
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
  "common.cancel": "Annuler",
  "settings.env.stacksDir": "Répertoire des stacks",
  "settings.env.dataDir": "Répertoire de données",
  "settings.env.publicHost": "Hôte public",
  "settings.env.auth": "Authentification",
  "settings.env.version": "Version",
  "dashboard.search": "Rechercher des apps…",
  "dashboard.showHidden": "Afficher les masqués ({{n}})",
  "dashboard.active": "actifs",
  "dashboard.inactive": "inactifs",
  "dashboard.exited": "arrêtés",
  "dashboard.customize": "Personnaliser",
  "stacks.new": "Nouveau",
  "stacks.deploy": "Déployer",
  "stacks.pull": "Pull",
  "stacks.restart": "Redémarrer",
  "stacks.stop": "Arrêter",
  "stacks.rename": "Renommer",
  "stacks.delete": "Supprimer",
  "stacks.validate": "Valider",
  "stacks.revert": "Rétablir",
  "stacks.logs": "Journaux",
  "stacks.follow": "Suivre",
  "stacks.enlarge": "Agrandir",
  "updates.checkNow": "Vérifier",
  "updates.checking": "Vérification…",
  "updates.allUpToDate": "Tout est à jour",
  "updates.ignore": "Ignorer",
  "updates.unignore": "Ne plus ignorer",
  "updates.selectAll": "Tout sélectionner",
  "updates.sectionAvailable": "Mise à jour disponible",
  "updates.sectionIgnored": "Ignorés",
  "updates.sectionUpToDate": "À jour",
  "updates.sectionOther": "Non vérifié / autres",
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
