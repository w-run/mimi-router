import PropTypes from 'prop-types';

// material-ui
import { useTheme } from '@mui/material/styles';
import { Box, ButtonBase, IconButton } from '@mui/material';

// project imports
import LogoSection from '../LogoSection';
import ProfileSection from './ProfileSection';
import ThemeButton from 'ui-component/ThemeButton';

// assets
import { IconMenu2 } from '@tabler/icons-react';

// ==============================|| MAIN NAVBAR / HEADER ||============================== //

const Header = ({ handleLeftDrawerToggle }) => {
  const theme = useTheme();

  return (
    <>
      {/* logo & toggler button */}
      <Box
        sx={{
          width: 228,
          display: 'flex',
          alignItems: 'center',
          [theme.breakpoints.down('md')]: {
            width: 'auto'
          }
        }}
      >
        <Box component="span" sx={{ display: { xs: 'none', md: 'flex' }, flexGrow: 1 ,  alignItems: 'center', justifyContent: 'center' }}>
          <LogoSection />
        </Box>
        <IconButton
          sx={{
            borderRadius: '12px',
            transition: 'all .2s ease-in-out',
            width: '40px',
            height: '40px',
            '&:hover': {
              background: theme.palette.primary.dark,
              color: theme.palette.primary.light
            }
          }}
          onClick={handleLeftDrawerToggle}
          color="inherit"
        >
          <IconMenu2 stroke={1.5} size="1.3rem" />
        </IconButton>
      </Box>

      <Box sx={{ flexGrow: 1 }} />
      <Box sx={{ flexGrow: 1 }} />
      <ThemeButton />
      <ProfileSection />
    </>
  );
};

Header.propTypes = {
  handleLeftDrawerToggle: PropTypes.func
};

export default Header;
