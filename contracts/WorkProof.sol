// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

/**
 * @title WorkProof
 * @dev 区块链上的工作量证明合约
 */
contract WorkProof {
    // ============ 数据结构 ============
    
    struct WorkRecord {
        address agent;          // Agent 地址
        string agentId;         // Agent ID (链下标识)
        uint256 timestamp;     // 时间戳
        uint256 tokensUsed;    // 消耗 token 数
        uint256 codeLines;     // 生成代码行数
        uint256 wordsWritten;  // 文字产出
        uint256 bugsFixed;     // 修复 bug 数
        uint256 value;        // 价值分数
        bytes32 proofHash;    // 工作证明哈希
        bool claimed;         // 是否已领取奖励
    }
    
    struct AgentStats {
        uint256 totalTasks;       // 总任务数
        uint256 completedTasks;   // 完成数
        uint256 totalValue;       // 总价值
        uint256 totalReward;      // 已领取奖励
        uint256 rank;             // 排名
    }
    
    // ============ 状态变量 ============
    
    mapping(bytes32 => WorkRecord) public workRecords;     // 工作记录
    mapping(address => AgentStats) public agentStats;     // Agent 统计
    mapping(uint256 => address) public topAgents;         // 排名
    
    uint256 public totalWorkValue;    // 全网总价值
    uint256 public totalRecords;      // 总记录数
    uint256 public rewardPool;        // 奖励池
    
    address public owner;
    uint256 public constant REWARD_PER_VALUE = 1e18; // 每单位价值对应奖励
    
    // ============ 新增：自动注销功能 ============
    uint256 public constant INACTIVE_DAYS = 730 days; // 2年无活动自动注销
    mapping(address => uint256) public lastActiveTime; // 最后活跃时间
    mapping(address => uint256) public lockedBalance; // 锁定的余额
    
    event AccountFrozen(address indexed agent, uint256 balance);
    event BalanceReleased(address indexed agent, uint256 amount);
    
    // ============ 事件 ============
    
    event WorkRecorded(
        bytes32 indexed recordId,
        address indexed agent,
        uint256 value,
        bytes32 proofHash
    );
    
    event RewardClaimed(
        address indexed agent,
        uint256 amount
    );
    
    // ============ 构造函数 ============
    
    constructor() {
        owner = msg.sender;
    }
    
    // ============ 核心函数 ============
    
    /**
     * @dev 记录工作证明
     */
    function recordWork(
        string calldata _agentId,
        uint256 _tokensUsed,
        uint256 _codeLines,
        uint256 _wordsWritten,
        uint256 _bugsFixed,
        bytes32 _proofHash
    ) external returns (bytes32) {
        // 计算价值
        uint256 value = calculateValue(
            _tokensUsed,
            _codeLines,
            _wordsWritten,
            _bugsFixed
        );
        
        // 生成记录ID
        bytes32 recordId = keccak256(
            abi.encodePacked(
                _agentId,
                block.timestamp,
                _proofHash
            )
        );
        
        // 存储记录
        workRecords[recordId] = WorkRecord({
            agent: msg.sender,
            agentId: _agentId,
            timestamp: block.timestamp,
            tokensUsed: _tokensUsed,
            codeLines: _codeLines,
            wordsWritten: _wordsWritten,
            bugsFixed: _bugsFixed,
            value: value,
            proofHash: _proofHash,
            claimed: false
        });
        
        // 更新统计
        AgentStats storage stats = agentStats[msg.sender];
        stats.totalTasks++;
        stats.completedTasks++;
        stats.totalValue += value;
        
        totalWorkValue += value;
        totalRecords++;
        
        // 更新排名
        _updateRank(msg.sender);
        
        emit WorkRecorded(recordId, msg.sender, value, _proofHash);
        
        return recordId;
    }
    
    /**
     * @dev 领取奖励
     */
    function claimReward(bytes32 _recordId) external {
        WorkRecord storage record = workRecords[_recordId];
        
        require(record.agent == msg.sender, "Not the owner");
        require(!record.claimed, "Already claimed");
        
        uint256 reward = record.value * REWARD_PER_VALUE / 1e6;
        require(rewardPool >= reward, "Insufficient reward pool");
        
        record.claimed = true;
        agentStats[msg.sender].totalReward += reward;
        rewardPool -= reward;
        
        // 发放代币 (需要集成 ERC20)
        // IERC20(rewardToken).transfer(msg.sender, reward);
        
        emit RewardClaimed(msg.sender, reward);
    }
    
    /**
     * @dev 计算价值
     */
    function calculateValue(
        uint256 _tokensUsed,
        uint256 _codeLines,
        uint256 _wordsWritten,
        uint256 _bugsFixed
    ) public pure returns (uint256) {
        // 代码行价值最高
        uint256 codeValue = _codeLines * 50;
        
        // 修复 bug 价值
        uint256 bugValue = _bugsFixed * 100;
        
        // 文字产出价值
        uint256 wordValue = _wordsWritten * 5;
        
        // token 消耗成本
        uint256 cost = _tokensUsed / 1000;
        
        // 净价值
        return codeValue + bugValue + wordValue - cost;
    }
    
    /**
     * @dev 获取 Agent 排名
     */
    function getAgentRank(address _agent) external view returns (uint256) {
        return agentStats[_agent].rank;
    }
    
    /**
     * @dev 获取 Top N Agents
     */
    function getTopAgents(uint256 _n) external view returns (address[] memory) {
        address[] memory result = new address[](_n);
        for (uint256 i = 0; i < _n; i++) {
            result[i] = topAgents[i];
        }
        return result;
    }
    
    // ============ 内部函数 ============
    
    function _updateRank(address _agent) internal {
        uint256 currentRank = agentStats[_agent].rank;
        uint256 newValue = agentStats[_agent].totalValue;
        
        // 简单排名更新 (实际需要更复杂的排序逻辑)
        if (currentRank == 0) {
            // 新 Agent
            for (uint256 i = 0; i < totalRecords; i++) {
                if (newValue > agentStats[topAgents[i]].totalValue) {
                    // 插入
                    for (uint256 j = totalRecords; j > i; j--) {
                        topAgents[j] = topAgents[j-1];
                    }
                    topAgents[i] = _agent;
                    agentStats[_agent].rank = i + 1;
                    return;
                }
            }
            topAgents[totalRecords] = _agent;
            agentStats[_agent].rank = totalRecords + 1;
        }
    }
    
    // ============ 管理函数 ============
    
    function depositReward() external payable {
        rewardPool += msg.value;
    }
    
    function withdraw() external {
        require(msg.sender == owner);
        payable(owner).transfer(address(this).balance);
    }
    
    // ============ 自动注销功能 ============
    
    /**
     * @dev 更新最后活跃时间 (每次有活动时调用)
     */
    function updateLastActive() external {
        lastActiveTime[msg.sender] = block.timestamp;
    }
    
    /**
     * @dev 检查并释放不活跃账户
     * @param _agent 要检查的账户地址
     * @return 释放的余额金额
     */
    function checkAndReleaseInactive(address _agent) external returns (uint256) {
        require(lastActiveTime[_agent] > 0, "Agent never active");
        
        uint256 inactivePeriod = block.timestamp - lastActiveTime[_agent];
        
        // 检查是否超过30天不活跃
        if (inactivePeriod > INACTIVE_DAYS) {
            uint256 balance = agentStats[_agent].totalReward;
            
            if (balance > 0) {
                // 锁定余额
                lockedBalance[_agent] = balance;
                
                // 清零奖励
                agentStats[_agent].totalReward = 0;
                
                // 释放到奖励池
                rewardPool += balance;
                
                emit BalanceReleased(_agent, balance);
                emit AccountFrozen(_agent, balance);
                
                return balance;
            }
        }
        
        return 0;
    }
    
    /**
     * @dev 批量检查多个账户
     * @param _agents 账户地址数组
     */
    function checkBatchInactive(address[] calldata _agents) external {
        for (uint256 i = 0; i < _agents.length; i++) {
            checkAndReleaseInactive(_agents[i]);
        }
    }
    
    /**
     * @dev 查询账户是否将被释放
     * @param _agent 账户地址
     */
    function willBeReleased(address _agent) external view returns (bool, uint256) {
        if (lastActiveTime[_agent] == 0) {
            return (false, 0);
        }
        
        uint256 inactivePeriod = block.timestamp - lastActiveTime[_agent];
        uint256 daysLeft = (INACTIVE_DAYS > inactivePeriod) 
            ? (INACTIVE_DAYS - inactivePeriod) / 86400 
            : 0;
        
        return (inactivePeriod > INACTIVE_DAYS, daysLeft);
    }
}
