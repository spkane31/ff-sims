import Link from 'next/link';

const Footer = () => {
  return (
    <footer className="bg-gray-100 dark:bg-gray-900 py-8 mt-auto">
      <div className="container mx-auto px-4">
        <div className="flex flex-col items-center space-y-4">
          {/* Main prominent joke text */}
          <div className="text-center">
            <h3 className="text-2xl md:text-3xl font-bold text-blue-600 dark:text-blue-400 mb-2">
              Powered by Male Friendship™
            </h3>
            <p className="text-sm text-gray-600 dark:text-gray-400 italic">
              Because nothing brings the boys together like arguing over waiver wire pickups
            </p>
          </div>
          
          {/* Secondary footer content */}
          <div className="flex flex-col md:flex-row justify-between items-center w-full pt-4 border-t border-gray-300 dark:border-gray-700">
            <div className="text-sm text-gray-600 dark:text-gray-400 mb-4 md:mb-0">
              © {new Date().getFullYear()} FF Sims. All rights reserved.
            </div>
            <div className="flex space-x-6">
              <Link href="/about" className="text-sm text-gray-600 dark:text-gray-400 hover:text-blue-600 dark:hover:text-blue-400">
                About
              </Link>
              <Link href="https://github.com/spkane31/ff-sims" className="text-sm text-gray-600 dark:text-gray-400 hover:text-blue-600 dark:hover:text-blue-400">
                Contact
              </Link>
            </div>
          </div>
        </div>
      </div>
    </footer>
  );
};

export default Footer;