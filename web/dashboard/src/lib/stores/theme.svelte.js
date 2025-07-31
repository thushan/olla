import { getContext, setContext } from 'svelte';

// Theme store using Svelte 5 runes
class ThemeStore {
  theme = $state('light');
  
  constructor() {
    // Check for saved theme preference or default to light mode
    if (typeof window !== 'undefined') {
      const saved = localStorage.getItem('theme');
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      
      this.theme = saved || (prefersDark ? 'dark' : 'light');
      this.applyTheme();
      
      // Listen for system theme changes
      window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
        if (!localStorage.getItem('theme')) {
          this.theme = e.matches ? 'dark' : 'light';
          this.applyTheme();
        }
      });
    }
  }
  
  toggle() {
    this.theme = this.theme === 'light' ? 'dark' : 'light';
    this.applyTheme();
    localStorage.setItem('theme', this.theme);
  }
  
  setTheme(newTheme) {
    this.theme = newTheme;
    this.applyTheme();
    localStorage.setItem('theme', this.theme);
  }
  
  applyTheme() {
    if (typeof document !== 'undefined') {
      const root = document.documentElement;
      if (this.theme === 'dark') {
        root.classList.add('dark');
      } else {
        root.classList.remove('dark');
      }
    }
  }
  
  get isDark() {
    return this.theme === 'dark';
  }
  
  get isLight() {
    return this.theme === 'light';
  }
}

const THEME_KEY = Symbol('theme');

export function setThemeStore() {
  return setContext(THEME_KEY, new ThemeStore());
}

export function getThemeStore() {
  return getContext(THEME_KEY);
}

// Export a singleton instance for non-component usage
export const themeStore = new ThemeStore();