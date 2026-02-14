import { inject, ref, type InjectionKey, type Ref } from "vue";

export type AdminLang = "zh" | "en";

export type AdminI18nContext = {
  language: Ref<AdminLang>;
  tx: (zh: string, en: string) => string;
};

const fallbackLanguage = ref<AdminLang>("en");
const fallbackContext: AdminI18nContext = {
  language: fallbackLanguage,
  tx: (_zh, en) => en
};

export const ADMIN_I18N_KEY: InjectionKey<AdminI18nContext> = Symbol("admin_i18n");

export function useAdminI18n(): AdminI18nContext {
  return inject(ADMIN_I18N_KEY, fallbackContext);
}
