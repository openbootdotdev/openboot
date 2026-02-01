import { writable } from 'svelte/store';
import { browser } from '$app/environment';

type Theme = 'light' | 'dark';
type ThemePreference = 'light' | 'dark' | 'system';

function getSystemTheme(): Theme {
	if (!browser) return 'dark';
	return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function applyTheme(theme: Theme) {
	if (browser) {
		document.documentElement.setAttribute('data-theme', theme === 'light' ? 'light' : '');
	}
}

function createThemeStore() {
	const getPreference = (): ThemePreference => {
		if (!browser) return 'system';
		return (localStorage.getItem('theme') as ThemePreference) || 'system';
	};

	const getEffectiveTheme = (pref: ThemePreference): Theme => {
		return pref === 'system' ? getSystemTheme() : pref;
	};

	const { subscribe, set } = writable<Theme>(getEffectiveTheme(getPreference()));

	if (browser) {
		window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
			if (getPreference() === 'system') {
				const newTheme = getSystemTheme();
				set(newTheme);
				applyTheme(newTheme);
			}
		});
	}

	return {
		subscribe,
		toggle: () => {
			if (!browser) return;
			const currentPref = getPreference();
			
			const nextPref: ThemePreference =
				currentPref === 'system' ? (getSystemTheme() === 'dark' ? 'light' : 'dark') :
				currentPref === 'light' ? 'dark' : 'system';

			localStorage.setItem('theme', nextPref);
			const effectiveTheme = getEffectiveTheme(nextPref);
			set(effectiveTheme);
			applyTheme(effectiveTheme);
		},
		init: () => {
			if (browser) {
				const effectiveTheme = getEffectiveTheme(getPreference());
				set(effectiveTheme);
				applyTheme(effectiveTheme);
			}
		},
		getPreference
	};
}

export const theme = createThemeStore();
