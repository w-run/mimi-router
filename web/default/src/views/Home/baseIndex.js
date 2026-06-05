import { Box, Typography, Button, Container, Stack } from '@mui/material';
import { GitHub } from '@mui/icons-material';
import Logo from 'ui-component/Logo';

const BaseIndex = () => (
  <Box
    sx={{
      minHeight: 'calc(100vh - 136px)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      bgcolor: 'background.default',
      px: 3,
      py: 10
    }}
  >
    <Container maxWidth="md">
      <Stack spacing={6} alignItems="center" sx={{ textAlign: 'center' }}>
        <Typography
          variant="h1"
          sx={{
            fontSize: { xs: '3rem', sm: '4.5rem', md: '6rem' },
            fontWeight: 700,
            letterSpacing: '-0.03em',
            lineHeight: 1.05
          }}
        >
          One API
        </Typography>
        <Typography
          variant="h4"
          color="text.secondary"
          sx={{
            fontSize: { xs: '1rem', sm: '1.25rem', md: '1.5rem' },
            fontWeight: 400,
            lineHeight: 1.7,
            maxWidth: 720
          }}
        >
          All in one 的 OpenAI 接口
          <br />
          整合各种 API 访问方式
          <br />
          一键部署，开箱即用
        </Typography>
        <Button
          variant="contained"
          size="large"
          startIcon={<GitHub />}
          href="https://github.com/w-run/one-api"
          target="_blank"
          sx={{
            borderRadius: 999,
            px: 4,
            py: 1.5,
            textTransform: 'none',
            fontSize: '1rem',
            fontWeight: 500
          }}
        >
          GitHub
        </Button>
      </Stack>
    </Container>
  </Box>
);

export default BaseIndex;
