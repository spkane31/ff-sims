import Link from "next/link";
import { useRouter } from "next/router";
import { useState } from "react";

const Header = () => {
  const router = useRouter();
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);

  const { leagueId } = router.query;
  const lid = leagueId as string | undefined;

  const navItems = lid
    ? [
        { name: "League", path: `/league/${lid}` },
        { name: "Simulations", path: `/league/${lid}/simulations` },
        { name: "Schedule", path: `/league/${lid}/schedule` },
        { name: "Teams", path: `/league/${lid}/teams` },
        { name: "Players", path: `/league/${lid}/players` },
        { name: "Transactions", path: `/league/${lid}/transactions` },
      ]
    : [
        { name: "Home", path: "/" },
      ];

  const toggleMobileMenu = () => {
    setIsMobileMenuOpen(!isMobileMenuOpen);
  };

  return (
    <header className="bg-gradient-to-r from-blue-600 to-blue-800 text-white shadow-md">
      <div className="container mx-auto px-4 py-4">
        <div className="flex justify-between items-center">
          <Link href={lid ? `/league/${lid}` : "/"} className="text-2xl font-bold">
            The League
          </Link>

          {/* Desktop Navigation */}
          <nav className="hidden md:block">
            <ul className="flex space-x-8">
              {navItems.map((item) => (
                <li key={item.path}>
                  <Link
                    href={item.path}
                    className={`px-3 py-2 rounded-md transition-colors duration-200 hover:bg-blue-700 ${
                      router.pathname === item.path || router.asPath.startsWith(item.path + "/")
                        ? "bg-blue-700 font-medium"
                        : "hover:bg-blue-700/70"
                    }`}
                  >
                    {item.name}
                  </Link>
                </li>
              ))}
              {lid && (
                <li>
                  <Link
                    href="/"
                    className="px-3 py-2 rounded-md transition-colors duration-200 hover:bg-blue-700/70 text-blue-200"
                  >
                    All Leagues
                  </Link>
                </li>
              )}
            </ul>
          </nav>

          {/* Mobile Menu Button */}
          <button
            className="md:hidden flex flex-col justify-center items-center w-8 h-8 space-y-1"
            onClick={toggleMobileMenu}
            aria-label="Toggle mobile menu"
          >
            <span className={`block w-6 h-0.5 bg-white transition-all duration-300 ${
              isMobileMenuOpen ? "rotate-45 translate-y-2" : ""
            }`}></span>
            <span className={`block w-6 h-0.5 bg-white transition-all duration-300 ${
              isMobileMenuOpen ? "opacity-0" : ""
            }`}></span>
            <span className={`block w-6 h-0.5 bg-white transition-all duration-300 ${
              isMobileMenuOpen ? "-rotate-45 -translate-y-2" : ""
            }`}></span>
          </button>
        </div>

        {/* Mobile Navigation */}
        <nav className={`md:hidden transition-all duration-300 overflow-hidden ${
          isMobileMenuOpen ? "max-h-96 opacity-100 mt-4" : "max-h-0 opacity-0"
        }`}>
          <ul className="flex flex-col space-y-2 py-2">
            {navItems.map((item) => (
              <li key={item.path}>
                <Link
                  href={item.path}
                  className={`block px-4 py-3 rounded-md transition-colors duration-200 hover:bg-blue-700 ${
                    router.asPath === item.path
                      ? "bg-blue-700 font-medium"
                      : "hover:bg-blue-700/70"
                  }`}
                  onClick={() => setIsMobileMenuOpen(false)}
                >
                  {item.name}
                </Link>
              </li>
            ))}
            {lid && (
              <li>
                <Link
                  href="/"
                  className="block px-4 py-3 rounded-md transition-colors duration-200 hover:bg-blue-700/70 text-blue-200"
                  onClick={() => setIsMobileMenuOpen(false)}
                >
                  All Leagues
                </Link>
              </li>
            )}
          </ul>
        </nav>
      </div>
    </header>
  );
};

export default Header;
