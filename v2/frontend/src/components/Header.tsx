import Link from "next/link";
import { useRouter } from "next/router";
import { useState, useEffect } from "react";
import { useLeague } from "../hooks/useLeague";
import { leaguesService, League } from "../services/leaguesService";

const Header = () => {
  const router = useRouter();
  const { leagueId, isLeagueContext } = useLeague();
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  const [isLeagueDropdownOpen, setIsLeagueDropdownOpen] = useState(false);
  const [leagues, setLeagues] = useState<League[]>([]);
  const [currentLeague, setCurrentLeague] = useState<League | null>(null);

  useEffect(() => {
    async function fetchLeagues() {
      try {
        const response = await leaguesService.getAllLeagues();
        setLeagues(response.leagues || []);

        if (leagueId && response.leagues) {
          const league = response.leagues.find((l) => l.id === leagueId);
          setCurrentLeague(league || null);
        }
      } catch (error) {
        console.error("Error fetching leagues:", error);
      }
    }

    fetchLeagues();
  }, [leagueId]);

  const navItems = isLeagueContext && leagueId
    ? [
        { name: "Home", path: `/league/${leagueId}` },
        { name: "Simulations", path: `/league/${leagueId}/simulations` },
        { name: "Schedule", path: `/league/${leagueId}/schedule` },
        { name: "Teams", path: `/league/${leagueId}/teams` },
        { name: "Players", path: `/league/${leagueId}/players` },
        { name: "Transactions", path: `/league/${leagueId}/transactions` },
      ]
    : [{ name: "Leagues", path: "/" }];

  const toggleMobileMenu = () => {
    setIsMobileMenuOpen(!isMobileMenuOpen);
  };

  const handleLeagueSwitch = (newLeagueId: string) => {
    setIsLeagueDropdownOpen(false);
    const currentPath = router.pathname;

    if (currentPath.includes("/league/[leagueId]")) {
      const newPath = currentPath.replace("[leagueId]", newLeagueId);
      const newQuery = { ...router.query, leagueId: newLeagueId };
      router.push({ pathname: newPath, query: newQuery });
    } else {
      router.push(`/league/${newLeagueId}`);
    }
  };

  return (
    <header className="bg-gradient-to-r from-blue-600 to-blue-800 text-white shadow-md">
      <div className="container mx-auto px-4 py-4">
        <div className="flex justify-between items-center">
          {isLeagueContext && currentLeague ? (
            <div className="relative">
              <button
                onClick={() => setIsLeagueDropdownOpen(!isLeagueDropdownOpen)}
                className="text-2xl font-bold flex items-center hover:text-blue-200 transition-colors"
              >
                {currentLeague.name}
                <svg
                  className={`w-5 h-5 ml-2 transition-transform ${
                    isLeagueDropdownOpen ? "rotate-180" : ""
                  }`}
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M19 9l-7 7-7-7"
                  />
                </svg>
              </button>

              {isLeagueDropdownOpen && leagues.length > 0 && (
                <div className="absolute top-full left-0 mt-2 w-64 bg-white rounded-md shadow-lg z-50">
                  <div className="py-1">
                    {leagues.map((league) => (
                      <button
                        key={league.id}
                        onClick={() => handleLeagueSwitch(league.id)}
                        className={`block w-full text-left px-4 py-2 text-sm ${
                          league.id === leagueId
                            ? "bg-blue-50 text-blue-700 font-medium"
                            : "text-gray-700 hover:bg-gray-100"
                        }`}
                      >
                        {league.name}
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          ) : (
            <Link href="/" className="text-2xl font-bold">
              Fantasy Football
            </Link>
          )}
          
          {/* Desktop Navigation */}
          <nav className="hidden md:block">
            <ul className="flex space-x-8">
              {navItems.map((item) => (
                <li key={item.path}>
                  <Link
                    href={item.path}
                    className={`px-3 py-2 rounded-md transition-colors duration-200 hover:bg-blue-700 ${
                      router.pathname === item.path
                        ? "bg-blue-700 font-medium"
                        : "hover:bg-blue-700/70"
                    }`}
                  >
                    {item.name}
                  </Link>
                </li>
              ))}
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
                    router.pathname === item.path
                      ? "bg-blue-700 font-medium"
                      : "hover:bg-blue-700/70"
                  }`}
                  onClick={() => setIsMobileMenuOpen(false)}
                >
                  {item.name}
                </Link>
              </li>
            ))}
          </ul>
        </nav>
      </div>
    </header>
  );
};

export default Header;
