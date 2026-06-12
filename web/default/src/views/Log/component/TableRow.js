import PropTypes from 'prop-types';

import { TableRow, TableCell } from '@mui/material';

import { timestamp2string, renderQuota } from 'utils/common';
import Label from 'ui-component/Label';
import LogType from '../type/LogType';

function renderType(type) {
  const typeOption = LogType[type];
  if (typeOption) {
    return (
      <Label variant="filled" color={typeOption.color}>
        {' '}
        {typeOption.text}{' '}
      </Label>
    );
  } else {
    return (
      <Label variant="filled" color="error">
        {' '}
        未知{' '}
      </Label>
    );
  }
}

// 渲染渠道单元格：优先显示名称，没有名称则回退到 ID。
// channelNameMap: Map<id, name>，由父组件传入。
function renderChannel(channelId, channelNameMap) {
  if (!channelId) return '';
  const name = channelNameMap && channelNameMap.get(channelId);
  if (name) {
    return (
      <Label color="default" variant="soft" title={`#${channelId}`}>
        {name}
      </Label>
    );
  }
  return String(channelId);
}

export default function LogTableRow({ item, userIsAdmin, channelNameMap }) {
  return (
    <>
      <TableRow tabIndex={item.id}>
        <TableCell>{timestamp2string(item.created_at)}</TableCell>

        {userIsAdmin && <TableCell>{renderChannel(item.channel, channelNameMap)}</TableCell>}
        {userIsAdmin && (
          <TableCell>
            <Label color="default" variant="outlined">
              {item.username}
            </Label>
          </TableCell>
        )}
        <TableCell>
          {item.token_name && (
            <Label color="default" variant="soft">
              {item.token_name}
            </Label>
          )}
        </TableCell>
        <TableCell>{renderType(item.type)}</TableCell>
        <TableCell>
          {item.model_name && (
            <Label color="primary" variant="outlined">
              {item.model_name}
            </Label>
          )}
        </TableCell>
        <TableCell>{item.prompt_tokens || ''}</TableCell>
        <TableCell>{item.completion_tokens || ''}</TableCell>
        <TableCell>{item.quota ? renderQuota(item.quota, 6) : ''}</TableCell>
        <TableCell>{item.content}</TableCell>
      </TableRow>
    </>
  );
}

LogTableRow.propTypes = {
  item: PropTypes.object,
  userIsAdmin: PropTypes.bool,
  channelNameMap: PropTypes.instanceOf(Map)
};
