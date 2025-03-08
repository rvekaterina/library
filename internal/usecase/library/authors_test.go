package library

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pkg/errors"
	"github.com/rvekaterina/library/internal/entity"
	"github.com/stretchr/testify/require"

	"github.com/golang/mock/gomock"
	"github.com/rvekaterina/library/internal/usecase/repository"
	mocks "github.com/rvekaterina/library/internal/usecase/repository/mocks"
	"go.uber.org/zap"
)

var (
	errOutbox     = errors.New("outbox error")
	errTransactor = errors.New("transactor error")
)

func setupAuthorUsecase(t *testing.T) (*libraryImpl, *mocks.MockAuthorRepository, *mocks.MockOutboxRepository, *mocks.MockTransactor) {
	t.Helper()
	controller := gomock.NewController(t)
	repo := mocks.NewMockAuthorRepository(controller)
	outboxRepo := mocks.NewMockOutboxRepository(controller)
	transactor := mocks.NewMockTransactor(controller)
	logger, _ := zap.NewProduction()
	impl := New(logger, repo, nil, outboxRepo, transactor)
	return impl, repo, outboxRepo, transactor
}

func TestCreateAuthor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		authorName  string
		setupMocks  func(*mocks.MockAuthorRepository, *mocks.MockOutboxRepository, *mocks.MockTransactor)
		expected    entity.Author
		expectedErr error
	}{
		{
			name:       "Success",
			authorName: "name",
			setupMocks: func(authorRepository *mocks.MockAuthorRepository, outboxRepository *mocks.MockOutboxRepository, transactor *mocks.MockTransactor) {
				author := entity.Author{ID: "123", Name: "name"}

				transactor.EXPECT().
					WithTx(context.Background(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
						return fn(ctx)
					}).
					Times(1)

				authorRepository.EXPECT().
					CreateAuthor(context.Background(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, a entity.Author) (entity.Author, error) {
						require.Equal(t, "name", a.Name)
						return author, nil
					}).
					Times(1)

				serialized, _ := json.Marshal(author)
				outboxRepository.EXPECT().
					SendMessage(
						context.Background(),
						repository.OutboxKindAuthor.String()+" "+author.ID,
						repository.OutboxKindAuthor,
						serialized,
					).
					Return(nil).
					Times(1)
			},
			expected: entity.Author{Name: "name", ID: "123"},
		},
		{
			name:       "Author repository error",
			authorName: "name",
			setupMocks: func(authorRepository *mocks.MockAuthorRepository, outboxRepository *mocks.MockOutboxRepository, transactor *mocks.MockTransactor) {
				transactor.EXPECT().
					WithTx(context.Background(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
						return fn(ctx)
					}).
					Times(1)

				authorRepository.EXPECT().
					CreateAuthor(gomock.Any(), entity.Author{Name: "name"}).
					Return(entity.Author{}, entity.ErrAuthorAlreadyExists).
					Times(1)

				outboxRepository.EXPECT().SendMessage(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedErr: entity.ErrAuthorAlreadyExists,
		},
		{
			name:       "Outbox error",
			authorName: "name",
			setupMocks: func(authorRepository *mocks.MockAuthorRepository, outboxRepository *mocks.MockOutboxRepository, transactor *mocks.MockTransactor) {
				author := entity.Author{Name: "name", ID: "123"}

				transactor.EXPECT().
					WithTx(context.Background(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
						return fn(ctx)
					}).
					Times(1)

				authorRepository.EXPECT().
					CreateAuthor(gomock.Any(), entity.Author{Name: "name"}).
					Return(author, nil).
					Times(1)

				serialized, _ := json.Marshal(author)
				outboxRepository.EXPECT().
					SendMessage(gomock.Any(), repository.OutboxKindAuthor.String()+" "+author.ID, repository.OutboxKindAuthor, serialized).
					Return(errOutbox).
					Times(1)
			},
			expectedErr: errOutbox,
		},
		{
			name:       "Transactor error",
			authorName: "name",
			setupMocks: func(authorRepository *mocks.MockAuthorRepository, outboxRepository *mocks.MockOutboxRepository, transactor *mocks.MockTransactor) {
				transactor.EXPECT().
					WithTx(context.Background(), gomock.Any()).
					Return(errTransactor)

				authorRepository.EXPECT().CreateAuthor(gomock.Any(), gomock.Any()).Times(0)
				outboxRepository.EXPECT().SendMessage(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectedErr: errTransactor,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			impl, repo, outboxRepo, transactor := setupAuthorUsecase(t)

			tc.setupMocks(repo, outboxRepo, transactor)

			res, err := impl.RegisterAuthor(context.Background(), tc.authorName)

			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
				require.EqualExportedValues(t, tc.expected, res)
			}
		})
	}
}

func TestGetAuthor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		authorID string
		err      error
	}{
		{
			name:     "Success",
			authorID: "1234",
			err:      nil,
		},
		{
			name:     "Error author not found",
			authorID: "1",
			err:      entity.ErrAuthorNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			impl, repo, _, _ := setupAuthorUsecase(t)

			repo.EXPECT().
				GetAuthor(context.Background(), tc.authorID).
				Return(entity.Author{ID: tc.authorID}, tc.err).
				Times(1)
			res, err := impl.GetAuthor(context.Background(), tc.authorID)
			require.Equal(t, tc.err, err)
			require.EqualExportedValues(t, entity.Author{ID: tc.authorID}, res)
		})
	}
}

func TestChangeAuthorInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		author entity.Author
		err    error
	}{
		{
			name:   "Success",
			author: entity.Author{ID: "1", Name: "name"},
			err:    nil,
		},
		{
			name:   "Error author not found",
			author: entity.Author{ID: "1", Name: "name"},
			err:    entity.ErrAuthorNotFound,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			impl, repo, _, _ := setupAuthorUsecase(t)

			repo.EXPECT().
				ChangeAuthorInfo(context.Background(), tc.author).
				Return(tc.err).
				Times(1)
			err := impl.ChangeAuthorInfo(context.Background(), tc.author.ID, tc.author.Name)
			require.Equal(t, tc.err, err)
		})
	}
}
