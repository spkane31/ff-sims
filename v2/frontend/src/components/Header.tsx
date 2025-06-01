import Link from 'next/link';
import { useRouter } from 'next/router';

const Header = () => {
  const router = useRouter();

  const navItems = [
    { name: 'Home', path: '/' },
    { name: 'Simulations', path: '/simulations' },
    { name: 'Schedule', path: '/schedule' },
    { name: 'Teams', path: '/teams' },
    { name: 'Transactions', path: '/transactions' },
  ];

  return (
    <header className="bg-gradient-to-r from-blue-600 to-blue-800 text-white shadow-md">
      <div className="container mx-auto px-4 py-4">
        <div className="flex flex-col md:flex-row justify-between items-center">
          <div className="text-2xl font-bold mb-4 md:mb-0">
             The League
          </div>
          <nav>
            <ul className="flex flex-wrap justify-center space-x-1 md:space-x-8">
              {navItems.map((item) => (
                <li key={item.path}>
                  <Link href={item.path}
                    className={`px-3 py-2 rounded-md transition-colors duration-200 hover:bg-blue-700 ${
                      router.pathname === item.path
                        ? 'bg-blue-700 font-medium'
                        : 'hover:bg-blue-700/70'
                    }`}
                  >
                    {item.name}
                  </Link>
                </li>
              ))}
            </ul>
          </nav>
        </div>
      </div>
    </header>
  );
};

export default Header;