import PropTypes from 'prop-types';
import { TableCell, TableHead, TableRow, TableSortLabel } from '@mui/material';

const headCells = [
  { id: 'id', label: 'ID', numeric: true },
  { id: 'name', label: '名称' },
  { id: 'group', label: '分组' },
  { id: 'type', label: '类型', numeric: true },
  { id: 'status', label: '状态', numeric: true },
  { id: 'response_time', label: '响应时间', numeric: true },
  { id: 'used_quota', label: '已消耗', numeric: true },
  { id: 'balance', label: '余额', numeric: true },
  { id: 'priority', label: '优先级', numeric: true },
  { id: 'actions', label: '操作', sortable: false }
];

const ChannelTableHead = ({ order, orderBy, onRequestSort }) => {
  const createSortHandler = (property) => (event) => {
    onRequestSort(event, property);
  };

  return (
    <TableHead>
      <TableRow>
        {headCells.map((headCell) => {
          const isActive = orderBy === headCell.id;
          const canSort = headCell.sortable !== false;
          return (
            <TableCell
              key={headCell.id}
              align={headCell.numeric ? 'right' : 'left'}
              sortDirection={isActive ? order : false}
            >
              {canSort ? (
                <TableSortLabel
                  active={isActive}
                  direction={isActive ? order : 'asc'}
                  onClick={createSortHandler(headCell.id)}
                >
                  {headCell.label}
                </TableSortLabel>
              ) : (
                headCell.label
              )}
            </TableCell>
          );
        })}
      </TableRow>
    </TableHead>
  );
};

ChannelTableHead.propTypes = {
  order: PropTypes.oneOf(['asc', 'desc']),
  orderBy: PropTypes.string,
  onRequestSort: PropTypes.func
};

export default ChannelTableHead;
