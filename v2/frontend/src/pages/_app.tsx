import type { AppProps } from 'next/app';
import '../styles/globals.css'; // Adjust path as needed for your global CSS

export default function App({ Component, pageProps }: AppProps) {
  return <Component {...pageProps} />;
}