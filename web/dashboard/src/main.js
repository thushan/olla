import './app.css';
import './components.css';
import { mount } from 'svelte';
import App from './App.svelte';

const target = document.getElementById('app');
if (!target) throw new Error('#app mount point missing');

const app = mount(App, { target });

export default app;
